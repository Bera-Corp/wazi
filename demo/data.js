window.WAZI_DEMO = {
 "generatedFrom": "scripts/context_engine.py (Phase 2 retrieval pipeline)",
 "evaluationTime": "2026-10-21T09:05:00Z",
 "request": {
  "tenantScope": "tenant_demo",
  "applicationId": "org.example.uptime-demo",
  "releaseVersion": "1.0.0",
  "operation": "install",
  "hostId": "host_demo01"
 },
 "records": {
  "manifest": {
   "appId": "org.example.uptime-demo",
   "name": "Uptime Demo",
   "version": "1.0.0",
   "reviewState": "signed",
   "digest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
  },
  "host": {
   "hostId": "host_demo01",
   "tenantScope": "tenant_demo",
   "os": {
    "family": "linux",
    "distribution": "ubuntu",
    "version": "24.04"
   },
   "architecture": "x86_64",
   "resources": {
    "cpuCores": 4,
    "ramMb": 8192,
    "storageFreeMb": 51200
   },
   "occupiedPorts": [
    22,
    80,
    443
   ]
  }
 },
 "planSteps": [
  {
   "stepId": "step_verify_manifest",
   "capability": "manifest.verify",
   "requiresApproval": false
  },
  {
   "stepId": "step_pull",
   "capability": "compose.pull",
   "requiresApproval": false
  },
  {
   "stepId": "step_checkpoint",
   "capability": "volume.backup",
   "requiresApproval": false
  },
  {
   "stepId": "step_inject_secret",
   "capability": "secret.inject",
   "requiresApproval": false
  },
  {
   "stepId": "step_up",
   "capability": "compose.up",
   "requiresApproval": false
  },
  {
   "stepId": "step_proxy",
   "capability": "proxy.configure",
   "requiresApproval": true
  },
  {
   "stepId": "step_health",
   "capability": "health.check",
   "requiresApproval": false
  }
 ],
 "approval": {
  "schemaVersion": "1.0.0",
  "approvalId": "appr_demo01",
  "caseId": "case_demo01",
  "planId": "plan_demo01",
  "planDigest": "sha256:ba161af0b8d8fb71f2542956a6fd2acf7abd368ac3cb74498a85f8b711d96cb4",
  "envelopeId": "env_demo01",
  "tenantScope": "tenant_demo",
  "hostId": "host_demo01",
  "applicationId": "org.example.uptime-demo",
  "action": "public-exposure",
  "decision": "approved",
  "approver": {
   "approverId": "user_demo",
   "role": "user"
  },
  "approvedAt": "2026-10-21T09:03:00Z",
  "expiresAt": "2026-10-21T09:30:00Z",
  "envelopeDigest": "sha256:d652131efbe34237cc664ec81741d5de7dc30f54eb3906ae8338f0e6f31f76a4"
 },
 "case": {
  "schemaVersion": "1.0.0",
  "caseId": "case_demo01",
  "tenantScope": "tenant_demo",
  "applicationId": "org.example.uptime-demo",
  "releaseVersion": "1.0.0",
  "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d",
  "hostId": "host_demo01",
  "envelopeId": "env_demo01",
  "planId": "plan_demo01",
  "operation": "install",
  "outcome": "success",
  "startedAt": "2026-10-21T09:03:00Z",
  "finishedAt": "2026-10-21T09:10:00Z",
  "durationSeconds": 420,
  "interventionCount": 0,
  "evidenceRefs": [
   "ev_demo_health01"
  ],
  "promotionState": "observed",
  "redaction": {
   "applied": true,
   "secretValuesPresent": false,
   "rawLogsIncluded": false
  }
 },
 "evidence": [
  {
   "schemaVersion": "1.0.0",
   "evidenceId": "ev_demo_health01",
   "deploymentCaseId": "case_demo01",
   "type": "health-check",
   "result": "pass",
   "artifact": {
    "digest": "sha256:4444444444444444444444444444444444444444444444444444444444444444",
    "uri": "s3://wazi-evidence-demo/case_demo01/health.json",
    "mediaType": "application/json"
   },
   "tool": {
    "name": "wazi-health-probe",
    "version": "0.1.0"
   },
   "commitOrReleaseDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d",
   "createdAt": "2026-10-21T09:10:00Z",
   "redaction": {
    "applied": true,
    "secretValuesPresent": false
   }
  }
 ],
 "metrics": {
  "fixtures": 17,
  "correct": 17,
  "hardEngineDecisionAccuracy": 1.0,
  "hardEngineFalseCompatibleRate": 0.0,
  "staleRejectionRecall": 1.0,
  "perFixture": [
   {
    "fixture": "eval_compatible",
    "expected": [
     "accept",
     "compatible"
    ],
    "got": [
     "accept",
     "compatible"
    ],
    "correct": true
   },
   {
    "fixture": "eval_filesystem-not-writable",
    "expected": [
     "reject",
     "filesystem-not-writable"
    ],
    "got": [
     "reject",
     "filesystem-not-writable"
    ],
    "correct": true
   },
   {
    "fixture": "eval_host-profile-expired",
    "expected": [
     "reject",
     "host-profile-expired"
    ],
    "got": [
     "reject",
     "host-profile-expired"
    ],
    "correct": true
   },
   {
    "fixture": "eval_insufficient-cpu",
    "expected": [
     "reject",
     "insufficient-cpu"
    ],
    "got": [
     "reject",
     "insufficient-cpu"
    ],
    "correct": true
   },
   {
    "fixture": "eval_insufficient-ram",
    "expected": [
     "reject",
     "insufficient-ram"
    ],
    "got": [
     "reject",
     "insufficient-ram"
    ],
    "correct": true
   },
   {
    "fixture": "eval_insufficient-storage",
    "expected": [
     "reject",
     "insufficient-storage"
    ],
    "got": [
     "reject",
     "insufficient-storage"
    ],
    "correct": true
   },
   {
    "fixture": "eval_licence-not-allowed",
    "expected": [
     "reject",
     "licence-not-allowed"
    ],
    "got": [
     "reject",
     "licence-not-allowed"
    ],
    "correct": true
   },
   {
    "fixture": "eval_missing-mandatory-fact",
    "expected": [
     "reject",
     "missing-mandatory-fact"
    ],
    "got": [
     "reject",
     "missing-mandatory-fact"
    ],
    "correct": true
   },
   {
    "fixture": "eval_occupied-port",
    "expected": [
     "reject",
     "occupied-port"
    ],
    "got": [
     "reject",
     "occupied-port"
    ],
    "correct": true
   },
   {
    "fixture": "eval_privileged-not-supported",
    "expected": [
     "reject",
     "privileged-not-supported"
    ],
    "got": [
     "reject",
     "privileged-not-supported"
    ],
    "correct": true
   },
   {
    "fixture": "eval_revoked-manifest",
    "expected": [
     "reject",
     "revoked-manifest"
    ],
    "got": [
     "reject",
     "revoked-manifest"
    ],
    "correct": true
   },
   {
    "fixture": "eval_stale-host-profile",
    "expected": [
     "reject",
     "stale-host-profile"
    ],
    "got": [
     "reject",
     "stale-host-profile"
    ],
    "correct": true
   },
   {
    "fixture": "eval_unsigned-manifest",
    "expected": [
     "reject",
     "unsigned-manifest"
    ],
    "got": [
     "reject",
     "unsigned-manifest"
    ],
    "correct": true
   },
   {
    "fixture": "eval_unsupported-compose-version",
    "expected": [
     "reject",
     "unsupported-compose-version"
    ],
    "got": [
     "reject",
     "unsupported-compose-version"
    ],
    "correct": true
   },
   {
    "fixture": "eval_unsupported-docker-version",
    "expected": [
     "reject",
     "unsupported-docker-version"
    ],
    "got": [
     "reject",
     "unsupported-docker-version"
    ],
    "correct": true
   },
   {
    "fixture": "eval_unsupported-os",
    "expected": [
     "reject",
     "unsupported-os"
    ],
    "got": [
     "reject",
     "unsupported-os"
    ],
    "correct": true
   },
   {
    "fixture": "eval_wrong-architecture",
    "expected": [
     "reject",
     "wrong-architecture"
    ],
    "got": [
     "reject",
     "wrong-architecture"
    ],
    "correct": true
   }
  ],
  "scenarios": 17,
  "endToEndAccuracy": 1.0,
  "falseAcceptRate": 0.0,
  "perScenario": [
   {
    "scenario": "compatible-live",
    "expected": "accept",
    "decision": "accept",
    "reasons": [],
    "blockedStage": "",
    "correct": true
   },
   {
    "scenario": "occupied-port",
    "expected": "not-accept",
    "decision": "reject",
    "reasons": [
     "occupied-port"
    ],
    "blockedStage": "compatibility",
    "correct": true
   },
   {
    "scenario": "revoked-manifest",
    "expected": "not-accept",
    "decision": "reject",
    "reasons": [
     "revoked-manifest"
    ],
    "blockedStage": "compatibility",
    "correct": true
   },
   {
    "scenario": "cross-tenant",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "tenant-mismatch"
    ],
    "blockedStage": "tenant",
    "correct": true
   },
   {
    "scenario": "missing-tenant-scope",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "tenant-scope-missing"
    ],
    "blockedStage": "bind",
    "correct": true
   },
   {
    "scenario": "tampered-procedure-only",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "no-eligible-procedure"
    ],
    "blockedStage": "procedure",
    "correct": true
   },
   {
    "scenario": "unsigned-procedure-only",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "no-eligible-procedure"
    ],
    "blockedStage": "procedure",
    "correct": true
   },
   {
    "scenario": "live-without-revocation",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "revocation-snapshot-required"
    ],
    "blockedStage": "revocation",
    "correct": true
   },
   {
    "scenario": "live-without-cursor",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "revocation-sequence-cursor-required"
    ],
    "blockedStage": "revocation",
    "correct": true
   },
   {
    "scenario": "revocation-sequence-rollback",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "revocation-sequence-rollback"
    ],
    "blockedStage": "revocation",
    "correct": true
   },
   {
    "scenario": "secret-values-in-host",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "secret-values-in-context"
    ],
    "blockedStage": "redaction",
    "correct": true
   },
   {
    "scenario": "shell-capability-envelope",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "envelope-schema-invalid"
    ],
    "blockedStage": "envelope",
    "correct": true
   },
   {
    "scenario": "wrong-release-binding",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "binding-mismatch"
    ],
    "blockedStage": "bind",
    "correct": true
   },
   {
    "scenario": "plan-artifact-mismatch",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "envelope-plan-mismatch"
    ],
    "blockedStage": "envelope",
    "correct": true
   },
   {
    "scenario": "plan-citation-unresolved",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "envelope-plan-mismatch"
    ],
    "blockedStage": "envelope",
    "correct": true
   },
   {
    "scenario": "plan-unresolved-questions",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "envelope-plan-mismatch"
    ],
    "blockedStage": "envelope",
    "correct": true
   },
   {
    "scenario": "live-failed-metadata",
    "expected": "not-accept",
    "decision": "blocked",
    "reasons": [
     "revocation-signature-digest"
    ],
    "blockedStage": "revocation",
    "correct": true
   }
  ]
 },
 "scenarios": [
  {
   "id": "compatible",
   "label": "Compatible host \u2014 install proceeds to approval",
   "result": {
    "decision": "accept",
    "reasons": [],
    "blockedStage": "",
    "revocationMode": "metadata-validated",
    "explanation": [
     "Bound org.example.uptime-demo 1.0.0 (install) to host_demo01 for tenant tenant_demo.",
     "Tenant isolation verified for tenant_demo.",
     "Redaction verified: no secret values or free-text secret patterns in context.",
     "Revocation snapshot metadata validated (sequence 1, payloadDigest/signer/algorithm checked, current; cryptographic Cosign verification not performed in this offline path).",
     "Hard compatibility engine accepted the manifest/host pair.",
     "Selected proc_install_uptime_demo v1.0.0 (score 95: exact-release-scope +50; review-state-signed +15; checkpoint-before-mutation +10; rollback-procedure-declared +10; verify-first-health-last +10).",
     "Cited 1 success and 5 failure cases from tenant tenant_demo across 16 failure signatures.",
     "Envelope assembled, rebound to the current records, and schema+binding validated before acceptance."
    ],
    "ranked": [
     {
      "procedureId": "proc_install_uptime_demo",
      "version": "1.0.0",
      "operation": "install",
      "compatible": true,
      "decision": "accept",
      "reasons": [],
      "score": 95,
      "scoreBreakdown": {
       "exact-release-scope": 50,
       "review-state-signed": 15,
       "checkpoint-before-mutation": 10,
       "rollback-procedure-declared": 10,
       "verify-first-health-last": 10
      },
      "explanation": "exact-release-scope +50; review-state-signed +15; checkpoint-before-mutation +10; rollback-procedure-declared +10; verify-first-health-last +10"
     },
     {
      "procedureId": "proc_rollback_uptime_demo",
      "version": "1.0.0",
      "operation": "rollback",
      "compatible": false,
      "decision": "reject",
      "reasons": [
       "procedure-operation-mismatch"
      ],
      "score": 0,
      "scoreBreakdown": {},
      "explanation": "no score (incompatible)"
     }
    ],
    "selectedProcedure": "proc_install_uptime_demo",
    "successCases": [
     {
      "caseId": "eval_compatible",
      "outcome": "success",
      "reason": "compatible",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     }
    ],
    "failureCases": [
     {
      "caseId": "eval_filesystem-not-writable",
      "outcome": "failure",
      "reason": "filesystem-not-writable",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_host-profile-expired",
      "outcome": "failure",
      "reason": "host-profile-expired",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T10:00:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-cpu",
      "outcome": "failure",
      "reason": "insufficient-cpu",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-ram",
      "outcome": "failure",
      "reason": "insufficient-ram",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-storage",
      "outcome": "failure",
      "reason": "insufficient-storage",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     }
    ],
    "failureSignatures": [
     {
      "signatureId": "sig_filesystem-not-writable",
      "reason": "filesystem-not-writable",
      "count": 1,
      "caseIds": [
       "eval_filesystem-not-writable"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_host-profile-expired",
      "reason": "host-profile-expired",
      "count": 1,
      "caseIds": [
       "eval_host-profile-expired"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-cpu",
      "reason": "insufficient-cpu",
      "count": 1,
      "caseIds": [
       "eval_insufficient-cpu"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-ram",
      "reason": "insufficient-ram",
      "count": 1,
      "caseIds": [
       "eval_insufficient-ram"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-storage",
      "reason": "insufficient-storage",
      "count": 1,
      "caseIds": [
       "eval_insufficient-storage"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_licence-not-allowed",
      "reason": "licence-not-allowed",
      "count": 1,
      "caseIds": [
       "eval_licence-not-allowed"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_missing-mandatory-fact",
      "reason": "missing-mandatory-fact",
      "count": 1,
      "caseIds": [
       "eval_missing-mandatory-fact"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_occupied-port",
      "reason": "occupied-port",
      "count": 1,
      "caseIds": [
       "eval_occupied-port"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_privileged-not-supported",
      "reason": "privileged-not-supported",
      "count": 1,
      "caseIds": [
       "eval_privileged-not-supported"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_revoked-manifest",
      "reason": "revoked-manifest",
      "count": 1,
      "caseIds": [
       "eval_revoked-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_stale-host-profile",
      "reason": "stale-host-profile",
      "count": 1,
      "caseIds": [
       "eval_stale-host-profile"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsigned-manifest",
      "reason": "unsigned-manifest",
      "count": 1,
      "caseIds": [
       "eval_unsigned-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-compose-version",
      "reason": "unsupported-compose-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-compose-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-docker-version",
      "reason": "unsupported-docker-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-docker-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-os",
      "reason": "unsupported-os",
      "count": 1,
      "caseIds": [
       "eval_unsupported-os"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_wrong-architecture",
      "reason": "wrong-architecture",
      "count": 1,
      "caseIds": [
       "eval_wrong-architecture"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     }
    ],
    "envelope": {
     "schemaVersion": "1.0.0",
     "envelopeId": "env_demo01",
     "tenantScope": "tenant_demo",
     "operation": "install",
     "deploymentMode": "deterministic",
     "applicationId": "org.example.uptime-demo",
     "releaseVersion": "1.0.0",
     "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d",
     "manifestReviewState": "signed",
     "hostProfileRef": {
      "recordId": "host_demo01",
      "digest": "sha256:a86e74528a4290aad012d6fbb105f253a3a40409af1093c689a58c3b6c374996",
      "version": "1.0.0"
     },
     "policyBundleRef": {
      "recordId": "pol_default",
      "digest": "sha256:63fb360fd98ef716b1332d40dec9339d53657181620ae678adecac59eb127162",
      "version": "1.0.0"
     },
     "procedureRef": {
      "recordId": "proc_install_uptime_demo",
      "digest": "sha256:23a5c359747e3f497dfb24cf5d254d54ebc46f3d54d12b5e3f709efa7e97709c",
      "version": "1.0.0"
     },
     "composeArtifactRef": {
      "recordId": "art_uptime_demo_1",
      "digest": "sha256:31745512abdc5399bf456743dd4d0a8d0958938c73a0ccf1cabb02304c5e0668",
      "version": "1.0.0"
     },
     "selectedSuccessCaseRefs": [
      {
       "recordId": "eval_compatible",
       "digest": "sha256:1afeb9edafb72321a19d170020774bbdceed942fc2aa88d984a7e8de02ec5023",
       "version": "1.0.0"
      }
     ],
     "selectedFailureCaseRefs": [
      {
       "recordId": "eval_filesystem-not-writable",
       "digest": "sha256:5cfb675c3d07870e95ab1e690a0775f151f51bc294f909c95d8c180d62ba4521",
       "version": "1.0.0"
      },
      {
       "recordId": "eval_host-profile-expired",
       "digest": "sha256:c597dadaaeb0591eb0b291b286c7bb24b30c8ba11873b42747ace971668d31d4",
       "version": "1.0.0"
      },
      {
       "recordId": "eval_insufficient-cpu",
       "digest": "sha256:c78449e38638ad452e3e7b632e49903344d9ef6d767f144537977e2ba68bec40",
       "version": "1.0.0"
      },
      {
       "recordId": "eval_insufficient-ram",
       "digest": "sha256:5e49717a6f70a8a8d8172c35ce61775f61da6849cdd32790a5bfd086663fefd4",
       "version": "1.0.0"
      },
      {
       "recordId": "eval_insufficient-storage",
       "digest": "sha256:fa2e27226fb66c54b876bdb607a58be4d821633f483e85a43792a0d9760aef47",
       "version": "1.0.0"
      }
     ],
     "secretReferences": [
      {
       "secretRef": "secret://deployment/app-secret",
       "injectionPoint": "compose environment APP_SECRET",
       "format": "opaque string, minimum 32 characters",
       "rotationRequired": false
      }
     ],
     "allowedCapabilities": [
      "manifest.verify",
      "compose.pull",
      "secret.inject",
      "compose.up",
      "proxy.configure",
      "health.check",
      "compose.down",
      "proxy.remove",
      "volume.backup"
     ],
     "forbiddenOperations": [
      "shell.exec",
      "volume.delete-without-checkpoint"
     ],
     "requiredApprovals": [
      "public-exposure"
     ],
     "verificationChecks": [
      {
       "id": "check_health",
       "type": "health",
       "target": "http://localhost:3000/health"
      }
     ],
     "rollbackStrategy": {
      "supported": true,
      "procedureRef": {
       "recordId": "proc_rollback_uptime_demo",
       "digest": "sha256:337ec77c0bbe93df3b39c5725ba5759a9197b2876154ca55fe81c1dbdbf6af51",
       "version": "1.0.0"
      },
      "checkpointRequired": false,
      "checkpoint": {
       "backupId": "bak_demo_checkpoint",
       "volume": "app-data"
      }
     },
     "citedSources": [
      {
       "recordId": "src_demo_repo",
       "contentHash": "sha256:1111111111111111111111111111111111111111111111111111111111111111"
      },
      {
       "recordId": "host_demo01",
       "contentHash": "sha256:a86e74528a4290aad012d6fbb105f253a3a40409af1093c689a58c3b6c374996"
      }
     ],
     "createdAt": "2026-10-21T09:05:00Z",
     "expiresAt": "2026-10-21T09:30:00Z"
    }
   }
  },
  {
   "id": "occupied-port",
   "label": "Port 3000 already occupied \u2014 denied at compatibility",
   "result": {
    "decision": "reject",
    "reasons": [
     "occupied-port"
    ],
    "blockedStage": "compatibility",
    "revocationMode": "metadata-validated",
    "explanation": [
     "Bound org.example.uptime-demo 1.0.0 (install) to host_demo01 for tenant tenant_demo.",
     "Tenant isolation verified for tenant_demo.",
     "Redaction verified: no secret values or free-text secret patterns in context.",
     "Revocation snapshot metadata validated (sequence 1, payloadDigest/signer/algorithm checked, current; cryptographic Cosign verification not performed in this offline path).",
     "Hard compatibility engine rejected: occupied-port. Ranking never ran."
    ],
    "ranked": [],
    "selectedProcedure": null,
    "successCases": [],
    "failureCases": [
     {
      "caseId": "eval_filesystem-not-writable",
      "outcome": "failure",
      "reason": "filesystem-not-writable",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_host-profile-expired",
      "outcome": "failure",
      "reason": "host-profile-expired",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T10:00:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-cpu",
      "outcome": "failure",
      "reason": "insufficient-cpu",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-ram",
      "outcome": "failure",
      "reason": "insufficient-ram",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-storage",
      "outcome": "failure",
      "reason": "insufficient-storage",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     }
    ],
    "failureSignatures": [
     {
      "signatureId": "sig_filesystem-not-writable",
      "reason": "filesystem-not-writable",
      "count": 1,
      "caseIds": [
       "eval_filesystem-not-writable"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_host-profile-expired",
      "reason": "host-profile-expired",
      "count": 1,
      "caseIds": [
       "eval_host-profile-expired"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-cpu",
      "reason": "insufficient-cpu",
      "count": 1,
      "caseIds": [
       "eval_insufficient-cpu"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-ram",
      "reason": "insufficient-ram",
      "count": 1,
      "caseIds": [
       "eval_insufficient-ram"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-storage",
      "reason": "insufficient-storage",
      "count": 1,
      "caseIds": [
       "eval_insufficient-storage"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_licence-not-allowed",
      "reason": "licence-not-allowed",
      "count": 1,
      "caseIds": [
       "eval_licence-not-allowed"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_missing-mandatory-fact",
      "reason": "missing-mandatory-fact",
      "count": 1,
      "caseIds": [
       "eval_missing-mandatory-fact"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_occupied-port",
      "reason": "occupied-port",
      "count": 1,
      "caseIds": [
       "eval_occupied-port"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_privileged-not-supported",
      "reason": "privileged-not-supported",
      "count": 1,
      "caseIds": [
       "eval_privileged-not-supported"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_revoked-manifest",
      "reason": "revoked-manifest",
      "count": 1,
      "caseIds": [
       "eval_revoked-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_stale-host-profile",
      "reason": "stale-host-profile",
      "count": 1,
      "caseIds": [
       "eval_stale-host-profile"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsigned-manifest",
      "reason": "unsigned-manifest",
      "count": 1,
      "caseIds": [
       "eval_unsigned-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-compose-version",
      "reason": "unsupported-compose-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-compose-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-docker-version",
      "reason": "unsupported-docker-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-docker-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-os",
      "reason": "unsupported-os",
      "count": 1,
      "caseIds": [
       "eval_unsupported-os"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_wrong-architecture",
      "reason": "wrong-architecture",
      "count": 1,
      "caseIds": [
       "eval_wrong-architecture"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     }
    ],
    "envelope": null
   }
  },
  {
   "id": "revoked",
   "label": "Manifest revoked \u2014 denied before ranking",
   "result": {
    "decision": "reject",
    "reasons": [
     "revoked-manifest"
    ],
    "blockedStage": "compatibility",
    "revocationMode": "fixture-no-record",
    "explanation": [
     "Bound org.example.uptime-demo 1.0.0 (install) to host_demo01 for tenant tenant_demo.",
     "Tenant isolation verified for tenant_demo.",
     "Redaction verified: no secret values or free-text secret patterns in context.",
     "Fixture mode: no revocation record supplied, defaulting to the caller's registry state. A live consumer must use live=True, which blocks in this situation.",
     "Hard compatibility engine rejected: revoked-manifest. Ranking never ran."
    ],
    "ranked": [],
    "selectedProcedure": null,
    "successCases": [],
    "failureCases": [
     {
      "caseId": "eval_filesystem-not-writable",
      "outcome": "failure",
      "reason": "filesystem-not-writable",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_host-profile-expired",
      "outcome": "failure",
      "reason": "host-profile-expired",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T10:00:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-cpu",
      "outcome": "failure",
      "reason": "insufficient-cpu",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-ram",
      "outcome": "failure",
      "reason": "insufficient-ram",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     },
     {
      "caseId": "eval_insufficient-storage",
      "outcome": "failure",
      "reason": "insufficient-storage",
      "applicationId": "org.example.uptime-demo",
      "releaseVersion": "1.0.0",
      "hostId": "host_demo01",
      "architecture": "x86_64",
      "osFamily": "linux",
      "tenantScope": "tenant_demo",
      "evaluationTime": "2026-10-21T09:05:00Z",
      "manifestDigest": "sha256:9011de079bbb6da635eb74b699dabb31ed87cc111e8992f46b4ba15e2744d49d"
     }
    ],
    "failureSignatures": [
     {
      "signatureId": "sig_filesystem-not-writable",
      "reason": "filesystem-not-writable",
      "count": 1,
      "caseIds": [
       "eval_filesystem-not-writable"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_host-profile-expired",
      "reason": "host-profile-expired",
      "count": 1,
      "caseIds": [
       "eval_host-profile-expired"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-cpu",
      "reason": "insufficient-cpu",
      "count": 1,
      "caseIds": [
       "eval_insufficient-cpu"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-ram",
      "reason": "insufficient-ram",
      "count": 1,
      "caseIds": [
       "eval_insufficient-ram"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_insufficient-storage",
      "reason": "insufficient-storage",
      "count": 1,
      "caseIds": [
       "eval_insufficient-storage"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_licence-not-allowed",
      "reason": "licence-not-allowed",
      "count": 1,
      "caseIds": [
       "eval_licence-not-allowed"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_missing-mandatory-fact",
      "reason": "missing-mandatory-fact",
      "count": 1,
      "caseIds": [
       "eval_missing-mandatory-fact"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_occupied-port",
      "reason": "occupied-port",
      "count": 1,
      "caseIds": [
       "eval_occupied-port"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_privileged-not-supported",
      "reason": "privileged-not-supported",
      "count": 1,
      "caseIds": [
       "eval_privileged-not-supported"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_revoked-manifest",
      "reason": "revoked-manifest",
      "count": 1,
      "caseIds": [
       "eval_revoked-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_stale-host-profile",
      "reason": "stale-host-profile",
      "count": 1,
      "caseIds": [
       "eval_stale-host-profile"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsigned-manifest",
      "reason": "unsigned-manifest",
      "count": 1,
      "caseIds": [
       "eval_unsigned-manifest"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-compose-version",
      "reason": "unsupported-compose-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-compose-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-docker-version",
      "reason": "unsupported-docker-version",
      "count": 1,
      "caseIds": [
       "eval_unsupported-docker-version"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_unsupported-os",
      "reason": "unsupported-os",
      "count": 1,
      "caseIds": [
       "eval_unsupported-os"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     },
     {
      "signatureId": "sig_wrong-architecture",
      "reason": "wrong-architecture",
      "count": 1,
      "caseIds": [
       "eval_wrong-architecture"
      ],
      "applicationIds": [
       "org.example.uptime-demo"
      ]
     }
    ],
    "envelope": null
   }
  }
 ]
};
