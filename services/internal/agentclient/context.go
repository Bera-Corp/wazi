package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/contracts"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

// ContextWorkerConfig is operator configuration, never supplied in an API body.
// Empty configuration leaves the context surface disabled.
type ContextWorkerConfig struct {
	PythonExecutable, WorkerPath, DatabasePath string
}

type ContextBridge struct {
	Worker                            ContextWorkerConfig
	QueueDirectory                    string
	Preflight                         ExecutionPreflightConfig
	Runtime                           *DockerPilotConfig
	Packages                          *PackageRegistry
	BudgetDigest, TenantScope, HostID string
	Probe                             func() (contracts.HostProfile, error)
}

func (c ContextWorkerConfig) Validate() error {
	for _, path := range []string{c.PythonExecutable, c.WorkerPath, c.DatabasePath} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("context worker paths must be absolute")
		}
	}
	for _, path := range []string{c.PythonExecutable, c.WorkerPath} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("context worker executable and source must be protected regular files")
		}
	}
	info, err := os.Lstat(filepath.Dir(c.DatabasePath))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return fmt.Errorf("context database requires an existing private directory")
	}
	if info, err := os.Lstat(c.DatabasePath); err == nil {
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
			return fmt.Errorf("context database must be a private regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

type boundedContextOutput struct{ bytes.Buffer }

func (b *boundedContextOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("context response too large")
	}
	return b.Buffer.Write(p)
}

// Call never invokes a shell or inherits the agent's credential environment.
func (c ContextWorkerConfig) Call(ctx context.Context, payload any) (json.RawMessage, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("invalid context input")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.PythonExecutable, "-I", c.WorkerPath, "--database", c.DatabasePath)
	// -I removes the script directory from sys.path; the entrypoint restores
	// only its operator-controlled source directory for sibling imports.
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = []string{"PYTHONUTF8=1"}
	for _, key := range []string{"SYSTEMROOT", "WINDIR", "TEMP", "TMP", "PATH"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	output := &boundedContextOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("context worker unavailable or rejected input")
	}
	if _, err := verify.ParseJSON(output.Bytes()); err != nil {
		return nil, fmt.Errorf("invalid context worker response")
	}
	return append(json.RawMessage(nil), output.Bytes()...), nil
}

// Retrieve verifies the same records as execution, but does not consume an
// approval or return an executable PreparedExecution to its caller.
func (b *ContextBridge) Retrieve(ctx context.Context, job PilotJob) (json.RawMessage, error) {
	for _, raw := range []*json.RawMessage{&job.Envelope, &job.Plan, &job.Artifact, &job.Budget, &job.Qualification} {
		value, err := verify.ParseJSON(*raw)
		if err != nil {
			return nil, err
		}
		canonical, err := verify.CanonicalJSON(value)
		if err != nil {
			return nil, err
		}
		*raw = canonical
	}
	if b.Probe == nil || b.TenantScope == "" || b.HostID == "" || b.BudgetDigest == "" {
		return nil, fmt.Errorf("context bridge identity and live probe are required")
	}
	if b.QueueDirectory != "" {
		if _, err := b.Worker.Call(ctx, map[string]any{"action": "sync", "queueDirectory": b.QueueDirectory}); err != nil {
			return nil, err
		}
	}
	budget, err := verify.ParseJSON(job.Budget)
	if err != nil {
		return nil, err
	}
	digest, err := verify.CanonicalDigest(budget)
	if b.Runtime == nil {
		return nil, fmt.Errorf("operator runtime configuration required")
	}
	boundary, expectedBudget, resolveErr := ResolveRuntimeBoundary(*b.Runtime, b.BudgetDigest, b.Packages, job.Artifact)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if err != nil || digest != expectedBudget {
		return nil, fmt.Errorf("owner-pinned budget required")
	}
	envelope, err := parseCanonicalObject(job.Envelope, "context envelope")
	if err != nil {
		return nil, err
	}
	plan, err := parseCanonicalObject(job.Plan, "context plan")
	if err != nil {
		return nil, err
	}
	envelopeID, _ := envelope["envelopeId"].(string)
	planID, _ := plan["planId"].(string)
	prepared, err := prepareExecution(ctx, b.Preflight, ExecutionPreflightRequest{
		JobID: job.JobID, CaseID: job.CaseID, EnvelopeID: envelopeID, PlanID: planID,
		Release: job.Release, RollbackProcedure: job.RollbackProcedure,
		EnvelopeRaw: job.Envelope, PlanRaw: job.Plan,
		ContributionBudgetRaw: job.Budget, ManagedVolumeQualificationRaw: job.Qualification,
	}, ExecutionResourceRecords{}, false)
	if err != nil {
		return nil, err
	}
	if prepared.Verified() || prepared.TenantScope() != b.TenantScope || prepared.HostID() != b.HostID {
		return nil, fmt.Errorf("read-only context identity mismatch")
	}
	if err := boundary.VerifyRootless(ctx); err != nil {
		return nil, err
	}
	if err := boundary.ValidateContext(prepared); err != nil {
		return nil, err
	}
	local, err := b.Probe()
	if err != nil {
		return nil, err
	}
	if local.TenantScope != b.TenantScope || local.HostID != b.HostID {
		return nil, fmt.Errorf("probe identity mismatch")
	}
	if err := VerifyHostCompatibility(prepared.Manifest(), local); err != nil {
		return nil, err
	}
	artifact, err := verify.ParseJSON(job.Artifact)
	if err != nil {
		return nil, err
	}
	artifactDigest, err := verify.CanonicalDigest(artifact)
	ref, ok := envelope["composeArtifactRef"].(map[string]any)
	if err != nil || !ok || ref["digest"] != artifactDigest {
		return nil, fmt.Errorf("artifact binding mismatch")
	}
	snapshot := map[string]any{}
	for key, raw := range map[string][]byte{
		"manifest": prepared.Manifest(), "procedure": prepared.Procedure(),
		"rollbackProcedure": prepared.RollbackProcedure(), "host": prepared.HostProfile(),
		"envelope": prepared.Envelope(), "plan": prepared.Plan(),
		"budget": job.Budget, "qualification": job.Qualification, "artifact": job.Artifact,
	} {
		if key == "rollbackProcedure" && len(raw) == 0 {
			continue
		}
		value, err := verify.ParseJSON(raw)
		if err != nil {
			return nil, err
		}
		snapshot[key] = value
	}
	policy := prepared.CapabilityPolicy().BindingSnapshot()
	snapshot["policyReference"] = map[string]any{"recordId": policy.BundleID, "version": policy.Version, "digest": policy.Digest, "tenantScope": policy.TenantScope}
	snapshot["policy"], err = verify.ParseJSON(prepared.CapabilityPolicy().Raw())
	if err != nil {
		return nil, err
	}
	cursorRaw, err := json.Marshal(b.Preflight.RevocationState.Cursor())
	if err != nil {
		return nil, err
	}
	snapshot["revocationCursor"], err = verify.ParseJSON(cursorRaw)
	if err != nil {
		return nil, err
	}
	localRaw, err := json.Marshal(local)
	if err != nil {
		return nil, err
	}
	snapshot["localHostObservation"], err = verify.ParseJSON(localRaw)
	if err != nil {
		return nil, err
	}
	return b.Worker.Call(ctx, map[string]any{"action": "retrieve", "evaluatedAt": time.Now().UTC().Format(time.RFC3339Nano), "snapshot": snapshot})
}

func (b *ContextBridge) Handler() http.Handler {
	mux := http.NewServeMux()
	respond := func(w http.ResponseWriter, raw []byte, err error) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(422)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "context-verification-rejected", "reason": contextRejectionReason(err)})
			return
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			w.WriteHeader(404)
		}
		_, _ = w.Write(raw)
	}
	mux.HandleFunc("GET /contexts/catalog", func(w http.ResponseWriter, r *http.Request) {
		raw, err := json.Marshal(b.Packages.Catalog())
		respond(w, raw, err)
	})
	mux.HandleFunc("POST /contexts/propose", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			respond(w, nil, err)
			return
		}
		if _, err := verify.ParseJSON(raw); err != nil {
			respond(w, nil, err)
			return
		}
		var request struct{ Project, Operation, BackupID string }
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			respond(w, nil, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		job, err := b.Propose(ctx, request.Project, request.Operation, request.BackupID)
		if err != nil {
			respond(w, nil, err)
			return
		}
		decision, err := b.Retrieve(ctx, job)
		if err != nil {
			respond(w, nil, err)
			return
		}
		output, err := json.Marshal(map[string]any{"job": job, "context": decision})
		respond(w, output, err)
	})
	mux.HandleFunc("POST /contexts/retrieve", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			respond(w, nil, err)
			return
		}
		if _, err := verify.ParseJSON(raw); err != nil {
			respond(w, nil, err)
			return
		}
		var job PilotJob
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var candidates []PilotJob
		if err := decoder.Decode(&job); err == nil {
			candidates = []PilotJob{job}
		} else {
			var query struct{ Candidates []PilotJob }
			decoder = json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&query); err != nil || len(query.Candidates) < 1 || len(query.Candidates) > 4 {
				respond(w, nil, fmt.Errorf("one to four candidates required"))
				return
			}
			candidates = query.Candidates
		}
		for _, candidate := range candidates {
			first := candidates[0].Release
			if candidate.Release.ApplicationID != first.ApplicationID || candidate.Release.ReleaseVersion != first.ReleaseVersion || candidate.Release.Operation != first.Operation {
				respond(w, nil, fmt.Errorf("candidates must share application, release and operation"))
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		accepted := []any{}
		rejected := []any{}
		// The bridge is bound to one tenant and host. No caller-controlled
		// tenant identity is used to query the case store.
		for index, candidate := range candidates {
			result, err := b.Retrieve(ctx, candidate)
			if err != nil {
				stage := "binding-or-resource-rejected"
				if strings.Contains(err.Error(), "revocation") || strings.Contains(err.Error(), "signed release") {
					stage = "signed-release-or-revocation-rejected"
				}
				rejected = append(rejected, map[string]any{"candidateIndex": index, "decision": "reject", "reason": stage})
				continue
			}
			value, err := verify.ParseJSON(result)
			if err != nil {
				respond(w, nil, err)
				return
			}
			accepted = append(accepted, value)
		}
		result, err := b.Worker.Call(ctx, map[string]any{"action": "search", "accepted": accepted, "rejected": rejected})
		respond(w, result, err)
	})
	mux.HandleFunc("GET /contexts/{contextID}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("contextID")
		if !regexp.MustCompile("^ctx_[0-9a-f]{64}$").MatchString(id) {
			respond(w, nil, fmt.Errorf("invalid context id"))
			return
		}
		raw, err := b.Worker.Call(r.Context(), map[string]any{"action": "inspect", "tenantScope": b.TenantScope, "contextId": id})
		respond(w, raw, err)
	})
	return mux
}

func contextRejectionReason(err error) string {
	message := strings.ToLower(err.Error())
	for _, group := range []struct {
		code  string
		words []string
	}{
		{"revocation-rejected", []string{"revocation", "revoked"}},
		{"signature-or-trust-rejected", []string{"signature", "signer", "signed release"}},
		{"stale-input-rejected", []string{"expired", "fresh", "future", "max age"}},
		{"resource-budget-rejected", []string{"budget", "resource", "memory", "quota", "storage", "cpu", "ram"}},
		{"port-unavailable", []string{"port"}},
		{"secret-reference-rejected", []string{"secret"}},
	} {
		for _, word := range group.words {
			if strings.Contains(message, word) {
				return group.code
			}
		}
	}
	return "binding-or-runtime-rejected"
}
