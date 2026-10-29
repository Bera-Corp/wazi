#!/usr/bin/env python3
"""Adversarial tests for offline S-Print compatibility, not model quality."""
import copy
import json
import unittest

import sprints


class SprintTests(unittest.TestCase):
    def setUp(self):
        self.record = json.loads((sprints.ROOT / "context/worlds/simulation-composition.json").read_text())

    def rejected(self, code):
        result = sprints.resolve(self.record)
        self.assertEqual(result["decision"], "reject")
        self.assertFalse(result["executionAuthorized"])
        self.assertTrue(any(code in error for error in result["errors"]), result["errors"])

    def test_composition_order_and_no_execution_authority(self):
        result = sprints.resolve(self.record)
        self.assertEqual(result["decision"], "compatible-offline")
        self.assertFalse(result["executionAuthorized"])
        self.assertEqual(result["evaluationStatus"], "not-run")
        self.assertEqual(result["loadOrder"], ["org.wazi.demo." + n for n in ("warehouse", "navigation", "planner")])
        self.assertEqual(result["reservedResources"]["ramMb"], 320)

    def test_missing_interface(self):
        self.record["runtime"]["provides"][0]["id"] = "wazi.demo.other"
        self.rejected("missing-or-incompatible-interface")

    def test_version_mismatch(self):
        self.record["packages"][0]["requires"][0]["version"] = "2.0.0"
        self.rejected("missing-or-incompatible-interface")

    def test_same_name_and_version_different_contract(self):
        self.record["packages"][0]["requires"][0]["contractDigest"] = "sha256:" + "0" * 64
        self.rejected("missing-or-incompatible-interface")

    def test_missing_dependency(self):
        self.record["packages"][0]["dependencies"] = [{"packageId": "org.wazi.absent", "version": "0.1.0", "digest": "sha256:" + "0" * 64}]
        self.rejected("missing-dependency")

    def test_digest_pinned_dependency(self):
        target = self.record["packages"][1]
        self.record["packages"][0]["dependencies"] = [{"packageId": target["packageId"], "version": target["version"], "digest": sprints.digest(target)}]
        self.assertEqual(sprints.resolve(self.record)["decision"], "compatible-offline")
        target["name"] = "Changed after pinning"
        self.rejected("dependency-pin-mismatch")

    def test_cycle(self):
        self.record["packages"][2]["requires"] = copy.deepcopy(self.record["packages"][0]["provides"])
        self.rejected("composition-cycle")

    def test_ambiguous_providers(self):
        self.record["packages"][1]["provides"] = copy.deepcopy(self.record["packages"][0]["provides"])
        self.rejected("ambiguous-interface-provider")

    def test_duplicate_packages(self):
        duplicate = copy.deepcopy(self.record["packages"][0])
        duplicate["name"] = "Alternate version with same identity"
        self.record["packages"].append(duplicate)
        self.rejected("duplicate-package-id")

    def test_memory_is_reserved_for_whole_composition(self):
        self.record["runtime"]["availableResources"]["ramMb"] = 319
        self.rejected("insufficient-composition-resources")

    def test_gpu_capacity(self):
        self.record["packages"][0]["resources"]["gpuVramMb"] = 1
        self.rejected("insufficient-composition-resources")

    def test_model_hash_mismatch(self):
        self.record["packages"][0]["baseModelDigest"] = "sha256:" + "0" * 64
        self.rejected("base-model-mismatch")

    def test_permission_not_granted(self):
        self.record["packages"][0]["permissions"].append("filesystem-read")
        self.rejected("permission-denied")

    def test_physical_control_is_unrepresentable_in_prototype(self):
        self.record["packages"][0]["permissions"].append("physical-control")
        self.rejected("schema:")

    def test_network_access_is_unrepresentable_in_prototype(self):
        self.record["packages"][0]["permissions"].append("network")
        self.rejected("schema:")

    def test_no_simulation_flag_downgrade(self):
        self.record["runtime"]["simulationOnly"] = False
        self.rejected("schema:")

    def test_training_is_metadata_only(self):
        self.record["packages"][0]["depth"] = "training"
        self.rejected("training-not-supported")

    def test_traversal_asset_path(self):
        for path in ("../outside", "/absolute", "assets/../../outside", "C:/host", "assets\\file", "assets/./file"):
            with self.subTest(path=path):
                self.record["packages"][0]["assets"][0]["path"] = path
                self.rejected("unsafe-asset-path")

    def test_missing_evaluation(self):
        del self.record["packages"][0]["evaluation"]
        self.rejected("schema:")

    def test_unknown_command_field(self):
        self.record["packages"][0]["command"] = "echo not-executed"
        self.rejected("schema:")

    def test_self_satisfying_interface(self):
        self.record["packages"][0]["requires"] = copy.deepcopy(self.record["packages"][0]["provides"])
        self.rejected("self-provided-requirement")


if __name__ == "__main__":
    unittest.main()
