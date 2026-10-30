package contracts

import "embed"

//go:embed schema/common.schema.json schema/app-manifest.schema.json schema/procedure.schema.json schema/revocation-record.schema.json schema/execution-context-envelope.schema.json schema/execution-context-envelope-v2.schema.json schema/deployment-plan.schema.json schema/approval-record.schema.json schema/contribution-budget.schema.json schema/managed-volume-qualification.schema.json schema/policy-bundle.schema.json
var embeddedSchemas embed.FS
