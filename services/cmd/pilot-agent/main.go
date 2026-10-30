// Command pilot-agent is the one-app development execution worker. Operator
// configuration pins identity, trust, budget, images and the rootless daemon.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/agentclient"
	"github.com/Bera-Corp/wazi/services/internal/operationjournal"
	"github.com/Bera-Corp/wazi/services/internal/pilotqueue"
	"github.com/Bera-Corp/wazi/services/internal/probe"
	"github.com/Bera-Corp/wazi/services/internal/verify"
)

type configuration struct {
	ControlPlaneURL, ServerName, SigningPolicyFile, BudgetDigest string
	Identity                                                     agentclient.Identity
	CapabilityPolicy                                             agentclient.CapabilityPolicyReference
	Runtime                                                      agentclient.DockerPilotConfig
}

// reconcileRequest is the operator's digest-bound observation for one
// ambiguous journal operation. Reconciliation never resumes execution:
// it records what was observed and blocks the operation so any further
// mutation needs a newly approved case.
type reconcileRequest struct {
	OperationID    string `json:"operationId"`
	StepID         string `json:"stepId"`
	ObservedState  string `json:"observedState"`
	SnapshotDigest string `json:"snapshotDigest"`
}

func reconcileHandler(journal *operationjournal.Journal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request reconcileRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid reconciliation request", http.StatusBadRequest)
			return
		}
		operation, err := journal.Reconcile(request.OperationID, request.StepID, request.ObservedState, request.SnapshotDigest)
		if err != nil {
			http.Error(w, "reconciliation rejected", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"operationId": operation.Identity.OperationID,
			"jobId":       operation.Identity.JobID,
			"state":       operation.State,
		})
	})
}

func main() {
	path := flag.String("config", "/etc/wazi-phase1/agent.json", "operator-managed pilot configuration")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *path); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, path string) error {
	ctx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 1<<20 {
		return fmt.Errorf("operator configuration is not a protected regular file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := verify.ParseJSON(raw); err != nil {
		return err
	}
	var config configuration
	if err := json.Unmarshal(raw, &config); err != nil {
		return err
	}
	policy, err := verify.LoadPolicyFile(config.SigningPolicyFile)
	if err != nil {
		return err
	}
	tlsConfig, err := agentclient.LoadTLS(config.Identity, config.ServerName)
	if err != nil {
		return err
	}
	state := config.Runtime.StateDirectory
	journal, err := operationjournal.New(filepath.Join(state, "journal"), nil)
	if err != nil {
		return err
	}
	defer journal.Close()
	cursor, err := agentclient.OpenRevocationStateStore(filepath.Join(state, "revocation"))
	if err != nil {
		return err
	}
	defer cursor.Close()
	q, err := pilotqueue.New(filepath.Join(state, "queue"), func(jobCtx context.Context, job agentclient.PilotJob) (any, error) {
		budgetValue, err := verify.ParseJSON(job.Budget)
		if err != nil {
			return nil, err
		}
		budgetDigest, err := verify.CanonicalDigest(budgetValue)
		if err != nil || budgetDigest != config.BudgetDigest {
			return nil, fmt.Errorf("request budget is not the owner-pinned budget")
		}
		envelopeValue, err := verify.ParseJSON(job.Envelope)
		if err != nil {
			return nil, err
		}
		envelope, ok := envelopeValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid envelope")
		}
		planValue, err := verify.ParseJSON(job.Plan)
		if err != nil {
			return nil, err
		}
		plan, ok := planValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid plan")
		}
		canonical := func(raw []byte) ([]byte, error) {
			value, err := verify.ParseJSON(raw)
			if err != nil {
				return nil, err
			}
			return verify.CanonicalJSON(value)
		}
		envelopeRaw, err := canonical(job.Envelope)
		if err != nil {
			return nil, err
		}
		planRaw, err := canonical(job.Plan)
		if err != nil {
			return nil, err
		}
		artifactRaw, err := canonical(job.Artifact)
		if err != nil {
			return nil, err
		}
		boundary, err := agentclient.NewDockerPilotBoundary(config.Runtime, artifactRaw)
		if err != nil {
			return nil, err
		}
		if err := boundary.VerifyRootless(jobCtx); err != nil {
			return nil, err
		}
		preflight := func(ctx context.Context) (agentclient.PreparedExecution, error) {
			request := agentclient.ExecutionPreflightRequest{JobID: job.JobID, CaseID: job.CaseID, Release: job.Release, RollbackProcedure: job.RollbackProcedure, EnvelopeRaw: envelopeRaw, PlanRaw: planRaw, Approvals: job.Approvals, ContributionBudgetRaw: job.Budget, ManagedVolumeQualificationRaw: job.Qualification}
			request.EnvelopeID, _ = envelope["envelopeId"].(string)
			request.PlanID, _ = plan["planId"].(string)
			prepared, err := agentclient.PrepareExecution(ctx, agentclient.ExecutionPreflightConfig{ControlPlaneURL: config.ControlPlaneURL, TLSConfig: tlsConfig, SigningPolicy: policy, CapabilityPolicy: config.CapabilityPolicy, RevocationState: cursor, RevocationFreshness: agentclient.RevocationFreshness{MaxAge: 5 * time.Minute}, ClockSkew: 5 * time.Second, ApprovalMaxValidity: 15 * time.Minute}, request, agentclient.ExecutionResourceRecords{})
			if err != nil {
				return prepared, err
			}
			// Re-probe actual local facts immediately before each mutation; never
			// substitute a stored control-plane profile for this observation.
			local, err := probe.BuildForHost(config.Runtime.TenantScope, config.Runtime.HostID)
			if err != nil {
				return agentclient.PreparedExecution{}, err
			}
			if err := agentclient.VerifyHostCompatibility(prepared.Manifest(), local); err != nil {
				return agentclient.PreparedExecution{}, err
			}
			return prepared, nil
		}
		executor, err := agentclient.New(agentclient.Config{Journal: journal, Boundary: boundary, Preflight: preflight})
		if err != nil {
			return nil, err
		}
		snapshot, err := executor.Execute(jobCtx, agentclient.Request{JobID: job.JobID, CaseID: job.CaseID})
		return snapshot, err
	})
	if err != nil {
		return err
	}
	// Publish a fresh initial profile, then keep the same bound observation
	// stable during a job. Further snapshots are explicitly requested by the
	// operator before constructing a new envelope and approval.
	mux := http.NewServeMux()
	mux.Handle("/jobs", q.Handler())
	mux.Handle("/jobs/", q.Handler())
	mux.Handle("POST /reconcile", reconcileHandler(journal))
	mux.HandleFunc("POST /profile", func(w http.ResponseWriter, r *http.Request) {
		profile, err := probe.BuildForHost(config.Runtime.TenantScope, config.Runtime.HostID)
		if err == nil {
			err = agentclient.SubmitProfile(r.Context(), config.ControlPlaneURL, tlsConfig, profile)
		}
		if err != nil {
			http.Error(w, "fresh profile unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(profile)
	})
	const socket = "/run/wazi-phase1/agent.sock"
	if _, err := os.Lstat(socket); err == nil {
		return fmt.Errorf("existing agent socket requires operator inspection before restart")
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(socket)
	if err := os.Chmod(socket, 0660); err != nil {
		return err
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	errorsChannel := make(chan error, 2)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); errorsChannel <- q.Run(ctx) }()
	go func() { errorsChannel <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err = <-errorsChannel:
	}
	cancelWorker()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	<-workerDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
