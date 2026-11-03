"""Machine-readable community contract inventory, separate from execution schemas."""
from .contracts import ENVIRONMENTS, OPERATIONS


def contracts():
    digest = {"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"}
    version = {"type": "string", "pattern": "^[0-9][A-Za-z0-9.+_-]{0,63}$"}
    identifier = {"type": "string", "pattern": "^[a-z][a-z0-9._-]{0,95}$"}
    environment = obj({"family": {"enum": sorted(ENVIRONMENTS)}, "architecture": {"enum": ["amd64", "arm64"]},
                       "desktopVersion": version, "guestVersion": version, "dockerVersion": version,
                       "composeVersion": version, "enforcement": {"enum": ["verified", "unverified", "unavailable"]}})
    report = obj({"schemaVersion": {"const": "1.0.0"}, "applicationId": identifier, "recipeDigest": digest,
                  "releaseVersion": version, "environment": environment, "operation": {"enum": sorted(OPERATIONS)},
                  "outcome": {"enum": ["success", "failure", "aborted"]},
                  "proofDigests": {"type": "array", "items": digest, "minItems": 1, "maxItems": 16, "uniqueItems": True},
                  "templateDigest": digest, "consentVersion": {"const": "1.0.0"}, "license": {"const": "Apache-2.0"}})
    operation_list = {"type": "array", "items": {"enum": sorted(OPERATIONS)}, "minItems": 1, "uniqueItems": True}
    recipe_fields = {"schemaVersion": {"const": "1.0.0"}, "recipeId": identifier,
                     "applicationId": identifier, "releaseVersion": version,
                     "revision": {"type": "integer", "minimum": 1, "maximum": 100000},
                     "license": {"const": "Apache-2.0"},
                     "authors": {"type": "array", "minItems": 1, "maxItems": 16,
                                 "items": {"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9-]{0,38}$"}},
                     "upstreamSources": {"type": "array", "minItems": 1, "maxItems": 16, "items": obj({
                         "repository": {"type": "string", "pattern": "^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$"},
                         "commit": {"type": "string", "pattern": "^[0-9a-f]{40}$"},
                         "license": {"type": "string", "pattern": "^[A-Za-z0-9.+-]{1,96}$"}})},
                     "inputs": {"type": "object", "maxProperties": 32, "propertyNames": identifier,
                                "additionalProperties": obj({"type": {"enum": ["instance", "port", "secret-reference"]}, "required": {"type": "boolean"}})},
                     "files": {"type": "object", "minProperties": 1, "maxProperties": 32, "required": ["manifest.json", "licenses.json"],
                               "propertyNames": {"type": "string", "pattern": "^[a-z][a-z0-9_-]{0,63}\\.json$"}, "additionalProperties": digest},
                     "environments": {"type": "array", "minItems": 1, "maxItems": 32, "items": obj({
                         "environment": environment, "operations": operation_list, "qualificationDigest": digest})},
                     "operations": operation_list,
                     "operationProcedures": {"type": "object", "propertyNames": {"enum": sorted(OPERATIONS)},
                                             "additionalProperties": {"type": "string", "pattern": "^procedure-[a-z0-9_-]+\\.json$"}},
                     "evidenceRefs": {"type": "array", "minItems": 1, "maxItems": 32, "items": digest}}
    signature = obj({"algorithm": {"const": "sigstore-cosign"}, "signedBy": {"type": "string", "minLength": 1},
                     "payloadDigest": digest, "value": {"type": "string", "minLength": 1}})
    signature["properties"]["signedAt"] = {"type": "string", "format": "date-time"}
    return {"$schema": "https://json-schema.org/draft/2020-12/schema", "$id": "https://wazi.dev/community/v1/contracts.schema.json",
            "title": "Wazi community contribution", "$ref": "#/$defs/contribution",
            "$defs": {"environment": environment, "contribution": report,
                      "recipeCandidate": obj(recipe_fields), "signedRecipe": obj(recipe_fields | {"signature": signature})},
            "$comment": "Community reports are untrusted observations. Runtime validation additionally binds procedure names, file hashes, manifest architecture/minimums, signature trust and exact environment scope."}


def obj(properties):
    return {"type": "object", "additionalProperties": False, "required": list(properties), "properties": properties}
