package contracts

import (
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const candidateQualificationSchemaID = "https://wazi.dev/schemas/v1/candidate-qualification.schema.json"

var (
	candidateQualificationSchemaOnce sync.Once
	candidateQualificationSchema     *jsonschema.Schema
	candidateQualificationSchemaErr  error
)

// ValidateCandidateQualification validates the closed signed runner receipt
// shape. Signature trust and candidate/tenant binding remain caller checks.
func ValidateCandidateQualification(raw []byte) error {
	return validateEmbeddedRecord(raw, loadCandidateQualificationSchema, func() error {
		return candidateQualificationSchemaErr
	}, "candidate qualification")
}

func loadCandidateQualificationSchema() *jsonschema.Schema {
	candidateQualificationSchemaOnce.Do(func() {
		candidateQualificationSchema, candidateQualificationSchemaErr = compileEmbeddedSchema(
			"schema/candidate-qualification.schema.json",
			candidateQualificationSchemaID,
		)
	})
	return candidateQualificationSchema
}
