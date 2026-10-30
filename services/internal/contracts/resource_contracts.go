package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Bera-Corp/wazi/services/internal/verify"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	contributionBudgetSchemaID         = "https://wazi.dev/schemas/v1/contribution-budget.schema.json"
	managedVolumeQualificationSchemaID = "https://wazi.dev/schemas/v1/managed-volume-qualification.schema.json"
	policyBundleSchemaID               = "https://wazi.dev/schemas/v1/policy-bundle.schema.json"
)

var (
	contributionBudgetSchemaOnce sync.Once
	contributionBudgetSchema     *jsonschema.Schema
	contributionBudgetSchemaErr  error

	managedVolumeQualificationSchemaOnce sync.Once
	managedVolumeQualificationSchema     *jsonschema.Schema
	managedVolumeQualificationSchemaErr  error

	policyBundleSchemaOnce sync.Once
	policyBundleSchema     *jsonschema.Schema
	policyBundleSchemaErr  error

	rfc3339WindowPattern = regexp.MustCompile(
		"^(\\d{4}-\\d{2}-\\d{2})[Tt](\\d{2}:\\d{2}:\\d{2})(?:\\.(\\d+))?([Zz]|[+-]\\d{2}:\\d{2})$",
	)
)

// ContributionResourceSet mirrors the resource quantities in the NODE-002
// contribution-budget schema. json.Number preserves decimal precision for
// exact capacity comparisons.
type ContributionResourceSet struct {
	CPUCores  json.Number `json:"cpuCores"`
	RAMMb     json.Number `json:"ramMb"`
	StorageMb json.Number `json:"storageMb"`
}

type ContributionAllocation struct {
	Role      string                  `json:"role"`
	Resources ContributionResourceSet `json:"resources"`
}

// ContributionBudget mirrors the NODE-002 v1 budget record. Decoding and
// validation do not authenticate its claimed approver or authorize execution.
type ContributionBudget struct {
	SchemaVersion string                   `json:"schemaVersion"`
	BudgetID      string                   `json:"budgetId"`
	TenantScope   string                   `json:"tenantScope"`
	HostID        string                   `json:"hostId"`
	ApprovalState string                   `json:"approvalState"`
	ApprovedBy    string                   `json:"approvedBy"`
	ApprovedAt    string                   `json:"approvedAt"`
	ValidFrom     string                   `json:"validFrom"`
	ValidUntil    string                   `json:"validUntil"`
	Physical      ContributionResourceSet  `json:"physical"`
	OwnerBudget   ContributionResourceSet  `json:"ownerBudget"`
	SystemReserve ContributionResourceSet  `json:"systemReserve"`
	Allocations   []ContributionAllocation `json:"allocations"`
}

type ManagedVolumeDaemon struct {
	ContextName   string `json:"contextName"`
	EndpointKind  string `json:"endpointKind"`
	ServerVersion string `json:"serverVersion"`
	RootDir       string `json:"rootDir"`
}

type ManagedVolumeStorage struct {
	ManagedVolumesCapable bool        `json:"managedVolumesCapable"`
	EnforcementMethod     string      `json:"enforcementMethod"`
	QuotaEnforceable      bool        `json:"quotaEnforceable"`
	StorageFreeMb         json.Number `json:"storageFreeMb"`
	FilesystemType        string      `json:"filesystemType"`
}

type ManagedVolume struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
}

// ManagedVolumeQualification mirrors the NODE-002/003 qualification record.
// Structural validation does not prove that the observation is live or that
// its evidence came from the selected daemon; consumers must establish both.
type ManagedVolumeQualification struct {
	SchemaVersion   string               `json:"schemaVersion"`
	QualificationID string               `json:"qualificationId"`
	TenantScope     string               `json:"tenantScope"`
	HostID          string               `json:"hostId"`
	ObservedAt      string               `json:"observedAt"`
	ExpiresAt       string               `json:"expiresAt"`
	Daemon          ManagedVolumeDaemon  `json:"daemon"`
	Storage         ManagedVolumeStorage `json:"storage"`
	Volumes         []ManagedVolume      `json:"volumes,omitempty"`
	Result          string               `json:"result"`
}

// DecodePolicyBundle applies the embedded schema and cross-field checks.
// Authentication is established by the caller using the configured control
// plane transport; schema validity alone does not make a policy trusted.
func DecodePolicyBundle(raw []byte) (map[string]any, error) {
	if err := validateEmbeddedRecord(raw, loadPolicyBundleSchema, func() error {
		return policyBundleSchemaErr
	}, "policy bundle"); err != nil {
		return nil, err
	}
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("decode policy bundle: %w", err)
	}
	bundle, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("policy bundle must be an object")
	}
	allowed := stringArraySet(bundle["allowedCapabilities"])
	denied := stringArraySet(bundle["deniedCapabilities"])
	for capability := range denied {
		if _, overlap := allowed[capability]; overlap {
			return nil, fmt.Errorf("policy bundle capability %q is both allowed and denied", capability)
		}
	}
	rules, ok := bundle["approvalRules"].([]any)
	if !ok {
		return nil, fmt.Errorf("policy bundle approvalRules must be an array")
	}
	seenActions := make(map[string]struct{}, len(rules))
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("policy bundle approval rule must be an object")
		}
		action, ok := rule["action"].(string)
		if !ok || action == "" {
			return nil, fmt.Errorf("policy bundle approval rule action is invalid")
		}
		if _, duplicate := seenActions[action]; duplicate {
			return nil, fmt.Errorf("policy bundle has duplicate approval rule for %q", action)
		}
		seenActions[action] = struct{}{}
	}
	return bundle, nil
}

// ValidatePolicyBundle applies schema and cross-field checks. The caller must
// still authenticate where the record came from before using it for execution.
func ValidatePolicyBundle(raw []byte) error {
	_, err := DecodePolicyBundle(raw)
	return err
}

// ValidateContributionBudget applies the embedded canonical schema and the
// semantic nesting rules that JSON Schema cannot express.
func ValidateContributionBudget(raw []byte) error {
	_, err := DecodeContributionBudget(raw)
	return err
}

// DecodeContributionBudget returns a schema-valid record only after enforcing
// distinct roles, exact allocation-plus-reserve arithmetic, and validity-window
// ordering. It does not verify owner identity, approval signatures, or freshness.
func DecodeContributionBudget(raw []byte) (ContributionBudget, error) {
	var record ContributionBudget
	if err := validateEmbeddedRecord(raw, loadContributionBudgetSchema, func() error {
		return contributionBudgetSchemaErr
	}, "contribution budget"); err != nil {
		return record, err
	}
	if err := decodeValidatedRecord(raw, &record, "contribution budget"); err != nil {
		return ContributionBudget{}, err
	}
	if err := validateContributionBudgetSemantics(record); err != nil {
		return ContributionBudget{}, err
	}
	return record, nil
}

// ValidateManagedVolumeQualification applies the embedded canonical schema and
// semantic freshness-window and usable-capacity checks. It does not perform a
// live daemon probe or authorize an executor to use the claim.
func ValidateManagedVolumeQualification(raw []byte) error {
	_, err := DecodeManagedVolumeQualification(raw)
	return err
}

func DecodeManagedVolumeQualification(raw []byte) (ManagedVolumeQualification, error) {
	var record ManagedVolumeQualification
	if err := validateEmbeddedRecord(raw, loadManagedVolumeQualificationSchema, func() error {
		return managedVolumeQualificationSchemaErr
	}, "managed-volume qualification"); err != nil {
		return record, err
	}
	if err := decodeValidatedRecord(raw, &record, "managed-volume qualification"); err != nil {
		return ManagedVolumeQualification{}, err
	}
	observed, err := parseWindowInstant(record.ObservedAt, "managed-volume qualification observedAt")
	if err != nil {
		return ManagedVolumeQualification{}, err
	}
	expires, err := parseWindowInstant(record.ExpiresAt, "managed-volume qualification expiresAt")
	if err != nil {
		return ManagedVolumeQualification{}, err
	}
	if compareWindowInstants(observed, expires) >= 0 {
		return ManagedVolumeQualification{}, fmt.Errorf("managed-volume qualification expiresAt must be after observedAt")
	}
	if record.Result == "qualified" {
		freeStorage, ok := new(big.Rat).SetString(string(record.Storage.StorageFreeMb))
		if !ok {
			return ManagedVolumeQualification{}, fmt.Errorf("managed-volume qualification: invalid storageFreeMb")
		}
		if freeStorage.Sign() <= 0 {
			return ManagedVolumeQualification{}, fmt.Errorf("managed-volume qualification: qualified with no free storage")
		}
	}
	return record, nil
}

// IsWindowActive compares RFC3339 endpoints at their exact fractional precision.
// The start is inclusive and the end is exclusive.
func IsWindowActive(start, end string, now time.Time) (bool, error) {
	startInstant, err := parseWindowInstant(start, "validFrom")
	if err != nil {
		return false, err
	}
	endInstant, err := parseWindowInstant(end, "validUntil")
	if err != nil {
		return false, err
	}
	if compareWindowInstants(startInstant, endInstant) >= 0 {
		return false, fmt.Errorf("validFrom must be before validUntil")
	}
	nowInstant, err := parseWindowInstant(now.UTC().Format(time.RFC3339Nano), "evaluation time")
	if err != nil {
		return false, err
	}
	return compareWindowInstants(startInstant, nowInstant) <= 0 && compareWindowInstants(nowInstant, endInstant) < 0, nil
}

func loadContributionBudgetSchema() *jsonschema.Schema {
	contributionBudgetSchemaOnce.Do(func() {
		contributionBudgetSchema, contributionBudgetSchemaErr = compileEmbeddedSchema(
			"schema/contribution-budget.schema.json",
			contributionBudgetSchemaID,
		)
	})
	return contributionBudgetSchema
}

func loadManagedVolumeQualificationSchema() *jsonschema.Schema {
	managedVolumeQualificationSchemaOnce.Do(func() {
		managedVolumeQualificationSchema, managedVolumeQualificationSchemaErr = compileEmbeddedSchema(
			"schema/managed-volume-qualification.schema.json",
			managedVolumeQualificationSchemaID,
		)
	})
	return managedVolumeQualificationSchema
}

func loadPolicyBundleSchema() *jsonschema.Schema {
	policyBundleSchemaOnce.Do(func() {
		policyBundleSchema, policyBundleSchemaErr = compileEmbeddedSchema(
			"schema/policy-bundle.schema.json",
			policyBundleSchemaID,
		)
	})
	return policyBundleSchema
}

func stringArraySet(value any) map[string]struct{} {
	items, ok := value.([]any)
	if !ok {
		return map[string]struct{}{}
	}
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		if capability, ok := item.(string); ok {
			result[capability] = struct{}{}
		}
	}
	return result
}

func decodeValidatedRecord[T any](raw []byte, record *T, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(record); err != nil {
		return fmt.Errorf("decode %s: %w", label, err)
	}
	return nil
}

func validateContributionBudgetSemantics(record ContributionBudget) error {
	roles := make(map[string]struct{}, len(record.Allocations))
	for _, allocation := range record.Allocations {
		if _, duplicate := roles[allocation.Role]; duplicate {
			return fmt.Errorf("contribution budget: duplicate allocation role %q", allocation.Role)
		}
		roles[allocation.Role] = struct{}{}
	}

	for _, resource := range []struct {
		name string
		get  func(ContributionResourceSet) json.Number
	}{
		{name: "cpuCores", get: func(set ContributionResourceSet) json.Number { return set.CPUCores }},
		{name: "ramMb", get: func(set ContributionResourceSet) json.Number { return set.RAMMb }},
		{name: "storageMb", get: func(set ContributionResourceSet) json.Number { return set.StorageMb }},
	} {
		allocated := new(big.Rat)
		for _, allocation := range record.Allocations {
			value, ok := new(big.Rat).SetString(string(resource.get(allocation.Resources)))
			if !ok {
				return fmt.Errorf("contribution budget: invalid %s allocation number", resource.name)
			}
			allocated.Add(allocated, value)
		}
		reserve, ok := new(big.Rat).SetString(string(resource.get(record.SystemReserve)))
		if !ok {
			return fmt.Errorf("contribution budget: invalid %s system reserve", resource.name)
		}
		budget, ok := new(big.Rat).SetString(string(resource.get(record.OwnerBudget)))
		if !ok {
			return fmt.Errorf("contribution budget: invalid %s owner budget", resource.name)
		}
		physical, ok := new(big.Rat).SetString(string(resource.get(record.Physical)))
		if !ok {
			return fmt.Errorf("contribution budget: invalid %s physical total", resource.name)
		}
		if new(big.Rat).Add(allocated, reserve).Cmp(budget) > 0 {
			return fmt.Errorf("contribution budget: allocations + systemReserve exceed ownerBudget for %s", resource.name)
		}
		if budget.Cmp(physical) > 0 {
			return fmt.Errorf("contribution budget: ownerBudget exceeds physical for %s", resource.name)
		}
	}

	start, err := parseWindowInstant(record.ValidFrom, "contribution budget validFrom")
	if err != nil {
		return err
	}
	end, err := parseWindowInstant(record.ValidUntil, "contribution budget validUntil")
	if err != nil {
		return err
	}
	if compareWindowInstants(start, end) >= 0 {
		return fmt.Errorf("contribution budget: validFrom must be before validUntil")
	}
	return nil
}

type windowInstant struct {
	utcSecond int64
	fraction  string
}

func parseWindowInstant(value, label string) (windowInstant, error) {
	matches := rfc3339WindowPattern.FindStringSubmatch(value)
	if matches == nil {
		return windowInstant{}, fmt.Errorf("%s must be an RFC3339 timestamp", label)
	}
	zone := matches[4]
	if strings.EqualFold(zone, "z") {
		zone = "Z"
	}
	parsed, err := time.Parse(time.RFC3339, matches[1]+"T"+matches[2]+zone)
	if err != nil {
		return windowInstant{}, fmt.Errorf("%s must be an RFC3339 timestamp: %w", label, err)
	}
	return windowInstant{
		utcSecond: parsed.UTC().Unix(),
		fraction:  strings.TrimRight(matches[3], "0"),
	}, nil
}

func compareWindowInstants(left, right windowInstant) int {
	if left.utcSecond < right.utcSecond {
		return -1
	}
	if left.utcSecond > right.utcSecond {
		return 1
	}
	length := len(left.fraction)
	if len(right.fraction) > length {
		length = len(right.fraction)
	}
	for i := 0; i < length; i++ {
		leftDigit, rightDigit := byte('0'), byte('0')
		if i < len(left.fraction) {
			leftDigit = left.fraction[i]
		}
		if i < len(right.fraction) {
			rightDigit = right.fraction[i]
		}
		if leftDigit < rightDigit {
			return -1
		}
		if leftDigit > rightDigit {
			return 1
		}
	}
	return 0
}
