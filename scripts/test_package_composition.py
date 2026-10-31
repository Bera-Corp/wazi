import copy
import unittest

import context_engine as engine
import package_composition as packages
import validate


class CompositionTests(unittest.TestCase):
    def setUp(self):
        self.manifest = engine.load_example_graph()["manifest"]
        self.manifest["appId"] = "org.postgresql.postgresql"
        self.digest = validate.canonical_digest(self.manifest)
        self.record = {"schemaVersion": "1.0.0", "compositionId": "composition_test",
                       "applicationId": self.manifest["appId"], "profile": "standalone",
                       "components": [{"id": "database", "role": "postgresql",
                                       "manifestRef": {"recordId": self.manifest["appId"], "version": self.manifest["version"], "digest": self.digest},
                                       "dependsOn": [], "limits": {"cpuCores": 2, "ramMb": 4096, "storageMb": 32768},
                                       "readiness": "postgres-authenticated", "secretRefs": ["secret://test/db"]}],
                       "recoveryGroups": [{"id": "db", "members": ["database"], "strategy": "database-logical"}]}
        self.available = {"cpuCores": 4, "ramMb": 8192, "storageMb": 65536}

    def check(self, record=None, available=None):
        return packages.validate_composition(record or self.record, {self.digest: self.manifest},
                                             available or self.available, ["secret://test/db"])

    def test_candidate_is_structurally_bound(self):
        self.assertEqual(self.check(), [])

    def test_dependencies_and_duplicate_ids(self):
        for dependency, expected in (("database", "dependency-cycle"), ("missing", "dependency-unresolved")):
            record = copy.deepcopy(self.record)
            record["components"][0]["dependsOn"] = [dependency]
            self.assertIn(expected, self.check(record))
        record = copy.deepcopy(self.record)
        record["components"].append(copy.deepcopy(record["components"][0]))
        self.assertIn("duplicate-component", self.check(record))

    def test_aggregate_limits_and_manifest_binding(self):
        available = dict(self.available, ramMb=1024)
        self.assertIn("aggregate-ramMb-exceeds-headroom", self.check(available=available))
        record = copy.deepcopy(self.record)
        record["components"][0]["manifestRef"]["digest"] = "sha256:" + "0" * 64
        self.assertIn("component-manifest-unresolved", self.check(record))
        record = copy.deepcopy(self.record)
        record["components"][0]["role"] = "mysql"
        self.assertIn("component-manifest-binding", self.check(record))

    def test_authentication_readiness_and_recovery(self):
        for field, value, expected in (("secretRefs", ["secret://other/db"], "undeclared-secret-reference"),
                                      ("secretRefs", [], "authenticated-service-secret-required"),
                                      ("readiness", "immich-http", "component-readiness-mismatch")):
            record = copy.deepcopy(self.record)
            record["components"][0][field] = value
            self.assertIn(expected, self.check(record))
        record = copy.deepcopy(self.record)
        record["recoveryGroups"] = []
        self.assertIn("recovery-coverage-missing", self.check(record))

    def test_no_free_form_commands_or_unqualified_immich(self):
        record = copy.deepcopy(self.record)
        record["components"][0]["command"] = "echo unreviewed"
        self.assertEqual(self.check(record), ["composition-schema-invalid"])
        record = copy.deepcopy(self.record)
        record["profile"] = "immich-no-ml"
        self.assertIn("immich-component-matrix", self.check(record))


if __name__ == "__main__":
    unittest.main()
