import hashlib
import unittest

import prepare_package_build as build


class PackageBuildTests(unittest.TestCase):
    def test_committed_sources_are_immutable_and_explicit(self):
        for profile, repository in build.REPOSITORIES.items():
            source = build.locked_source(profile)
            self.assertEqual(source["repository"], repository)
            self.assertFalse(source["commitSignatureVerified"])
            self.assertEqual(len(source["dockerfileSha256"]), 64)

    def test_only_exact_recipe_gets_pinned(self):
        raw = b"FROM debian:test\nRUN true\n"
        source = {"dockerfileSha256": hashlib.sha256(raw).hexdigest(),
                  "baseTag": "debian:test", "baseImage": "debian@sha256:" + "a" * 64}
        patched = build.patch_recipe(raw, source)
        self.assertIn(source["baseImage"].encode(), patched)
        self.assertTrue(patched.endswith(b"RUN true\n"))
        with self.assertRaisesRegex(ValueError, "digest-mismatch"):
            build.patch_recipe(raw + b"RUN changed\n", source)
        multiple = raw + b"FROM debian:test\n"
        source["dockerfileSha256"] = hashlib.sha256(multiple).hexdigest()
        with self.assertRaisesRegex(ValueError, "base-surface-changed"):
            build.patch_recipe(multiple, source)


if __name__ == "__main__":
    unittest.main()
