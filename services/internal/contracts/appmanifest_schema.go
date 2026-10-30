package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/Bera-Corp/wazi/services/internal/verify"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	commonSchemaID              = "https://wazi.dev/schemas/v1/common.schema.json"
	appManifestSchemaID         = "https://wazi.dev/schemas/v1/app-manifest.schema.json"
	procedureSchemaID           = "https://wazi.dev/schemas/v1/procedure.schema.json"
	revocationSchemaID          = "https://wazi.dev/schemas/v1/revocation-record.schema.json"
	executionEnvelopeSchemaID   = "https://wazi.dev/schemas/v1/execution-context-envelope.schema.json"
	executionEnvelopeV2SchemaID = "https://wazi.dev/schemas/v2/execution-context-envelope.schema.json"
	deploymentPlanSchemaID      = "https://wazi.dev/schemas/v1/deployment-plan.schema.json"
	approvalRecordSchemaID      = "https://wazi.dev/schemas/v1/approval-record.schema.json"
)

var (
	appManifestSchemaOnce         sync.Once
	appManifestSchema             *jsonschema.Schema
	appManifestSchemaErr          error
	procedureSchemaOnce           sync.Once
	procedureSchema               *jsonschema.Schema
	procedureSchemaErr            error
	revocationSchemaOnce          sync.Once
	revocationSchema              *jsonschema.Schema
	revocationSchemaErr           error
	executionEnvelopeSchemaOnce   sync.Once
	executionEnvelopeSchema       *jsonschema.Schema
	executionEnvelopeSchemaErr    error
	executionEnvelopeV2SchemaOnce sync.Once
	executionEnvelopeV2Schema     *jsonschema.Schema
	executionEnvelopeV2SchemaErr  error
	deploymentPlanSchemaOnce      sync.Once
	deploymentPlanSchema          *jsonschema.Schema
	deploymentPlanSchemaErr       error
	approvalRecordSchemaOnce      sync.Once
	approvalRecordSchema          *jsonschema.Schema
	approvalRecordSchemaErr       error
)

// ValidateAppManifest validates a strict JSON value against the canonical
// Phase 0 app-manifest contract. The schemas are embedded so the running
// control plane does not depend on its working directory. Their checked-in
// copies are compared with the root schemas by TestEmbeddedSchemasMatchSource.
func ValidateAppManifest(raw []byte) error {
	return validateEmbeddedRecord(raw, loadAppManifestSchema, func() error { return appManifestSchemaErr }, "app manifest")
}

// ValidateProcedure validates a strict JSON value against the canonical
// Phase 0 procedure schema. Signature verification remains a separate,
// mandatory registry step.
func ValidateProcedure(raw []byte) error {
	return validateEmbeddedRecord(raw, loadProcedureSchema, func() error { return procedureSchemaErr }, "procedure")
}

// ValidateRevocationRecord validates the signed revocation-list contract.
// Signature verification and freshness/sequence checks are separate,
// mandatory trust-boundary steps.
func ValidateRevocationRecord(raw []byte) error {
	return validateEmbeddedRecord(raw, loadRevocationSchema, func() error { return revocationSchemaErr }, "revocation record")
}

// ValidateExecutionContextEnvelope validates the typed operation context.
// Semantic binding to signed records and caller identity is performed by the
// agent preflight, not by this structural schema check.
func ValidateExecutionContextEnvelope(raw []byte) error {
	return validateEmbeddedRecord(raw, loadExecutionEnvelopeSchema, func() error { return executionEnvelopeSchemaErr }, "execution context envelope")
}

// ValidateExecutionContextEnvelopeV2 validates the resource-bound Phase 1
// execution envelope. Version 1 remains unchanged for Phase 0 compatibility.
func ValidateExecutionContextEnvelopeV2(raw []byte) error {
	return validateEmbeddedRecord(raw, loadExecutionEnvelopeV2Schema, func() error { return executionEnvelopeV2SchemaErr }, "execution context envelope v2")
}

// ValidateDeploymentPlan validates the closed typed plan. Semantic binding to
// the envelope and signed procedures is performed by the agent preflight.
func ValidateDeploymentPlan(raw []byte) error {
	return validateEmbeddedRecord(raw, loadDeploymentPlanSchema, func() error { return deploymentPlanSchemaErr }, "deployment plan")
}

// ValidateApprovalRecord applies the Phase 1 approval-record schema. Issuer
// authentication, case durability, and single-use consumption are separate
// semantic checks.
func ValidateApprovalRecord(raw []byte) error {
	return validateEmbeddedRecord(raw, loadApprovalRecordSchema, func() error { return approvalRecordSchemaErr }, "approval record")
}

func validateEmbeddedRecord(raw []byte, loadSchema func() *jsonschema.Schema, loadError func() error, label string) error {
	value, err := verify.ParseJSON(raw)
	if err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	schema := loadSchema()
	if schemaErr := loadError(); schemaErr != nil {
		return fmt.Errorf("load %s schema: %w", label, schemaErr)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("%s does not satisfy its schema: %w", label, err)
	}
	return nil
}

func loadAppManifestSchema() *jsonschema.Schema {
	appManifestSchemaOnce.Do(func() {
		appManifestSchema, appManifestSchemaErr = compileEmbeddedSchema("schema/app-manifest.schema.json", appManifestSchemaID)
	})
	return appManifestSchema
}

func loadProcedureSchema() *jsonschema.Schema {
	procedureSchemaOnce.Do(func() {
		procedureSchema, procedureSchemaErr = compileEmbeddedSchema("schema/procedure.schema.json", procedureSchemaID)
	})
	return procedureSchema
}

func loadRevocationSchema() *jsonschema.Schema {
	revocationSchemaOnce.Do(func() {
		revocationSchema, revocationSchemaErr = compileEmbeddedSchema("schema/revocation-record.schema.json", revocationSchemaID)
	})
	return revocationSchema
}

func loadExecutionEnvelopeSchema() *jsonschema.Schema {
	executionEnvelopeSchemaOnce.Do(func() {
		executionEnvelopeSchema, executionEnvelopeSchemaErr = compileEmbeddedSchema("schema/execution-context-envelope.schema.json", executionEnvelopeSchemaID)
	})
	return executionEnvelopeSchema
}

func loadExecutionEnvelopeV2Schema() *jsonschema.Schema {
	executionEnvelopeV2SchemaOnce.Do(func() {
		executionEnvelopeV2Schema, executionEnvelopeV2SchemaErr = compileEmbeddedSchema("schema/execution-context-envelope-v2.schema.json", executionEnvelopeV2SchemaID)
	})
	return executionEnvelopeV2Schema
}

func loadDeploymentPlanSchema() *jsonschema.Schema {
	deploymentPlanSchemaOnce.Do(func() {
		deploymentPlanSchema, deploymentPlanSchemaErr = compileEmbeddedSchema("schema/deployment-plan.schema.json", deploymentPlanSchemaID)
	})
	return deploymentPlanSchema
}

func loadApprovalRecordSchema() *jsonschema.Schema {
	approvalRecordSchemaOnce.Do(func() {
		approvalRecordSchema, approvalRecordSchemaErr = compileEmbeddedSchema("schema/approval-record.schema.json", approvalRecordSchemaID)
	})
	return approvalRecordSchema
}

func compileEmbeddedSchema(name, schemaID string) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	for _, resource := range []struct {
		name string
		id   string
	}{
		{name: "schema/common.schema.json", id: commonSchemaID},
		{name: name, id: schemaID},
	} {
		raw, err := embeddedSchemas.ReadFile(resource.name)
		if err != nil {
			return nil, err
		}
		var document any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&document); err != nil {
			return nil, err
		}
		if err := compiler.AddResource(resource.id, document); err != nil {
			return nil, err
		}
	}
	return compiler.Compile(schemaID)
}
