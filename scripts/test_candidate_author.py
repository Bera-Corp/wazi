import copy
import hashlib
import tempfile
import unittest
from pathlib import Path

import candidate_author as author
import context_engine as engine


class CandidateAuthorTests(unittest.TestCase):
    def setUp(self):
        self.manifest = engine.load_example_graph()["manifest"]
        self.manifest.pop("signature", None)
        self.manifest["reviewState"] = "candidate"
        text = "Pinned upstream installation documentation for this test."
        source = {"sourceId": "source_test", "url": "https://upstream.example/revision/docs",
                  "text": text, "contentHash": "sha256:" + hashlib.sha256(text.encode()).hexdigest()}
        self.manifest["provenance"]["sourceId"] = source["sourceId"]
        self.manifest["provenance"]["contentHash"] = source["contentHash"]
        self.output = {"manifest": self.manifest, "citations": [source["sourceId"]]}
        self.calls = []
        self.reservations = []
        self.args = dict(application_id=self.manifest["appId"], release_version=self.manifest["version"],
                         sources=[source], approved_images=self.manifest["release"]["images"],
                         reserve_budget=self.reservations.append,
                         pricing={"verified": True, "inputUsdPerMillion": "1", "outputUsdPerMillion": "2"})

    def provider(self, messages, max_tokens):
        self.calls.append(messages)
        return {"output": self.output, "model": "test-model", "usage": {"prompt_tokens": 100, "completion_tokens": 100}}

    def test_candidate_only_and_cost_reservation(self):
        result = author.author_candidate(self.provider, **self.args)
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(len(self.reservations), 1)
        self.assertEqual(result["manifest"]["reviewState"], "candidate")
        self.assertNotIn("signature", result["manifest"])
        self.assertIn("human-review", result["state"])

    def test_setup_and_tampered_sources_stop_before_call(self):
        for change in ({"pricing": {"verified": False}}, {"sources": []}):
            with self.assertRaises(ValueError):
                author.author_candidate(self.provider, **dict(self.args, **change))
        bad_sources = copy.deepcopy(self.args["sources"])
        bad_sources[0]["text"] += " changed"
        with self.assertRaisesRegex(ValueError, "source-pinning"):
            author.author_candidate(self.provider, **dict(self.args, sources=bad_sources))
        self.assertEqual(self.calls, [])
        self.assertEqual(self.reservations, [])

    def test_promotion_and_unapproved_images_rejected(self):
        self.output["manifest"]["reviewState"] = "reviewed"
        with self.assertRaisesRegex(ValueError, "candidate-cannot-publish"):
            author.author_candidate(self.provider, **self.args)
        self.output["manifest"]["reviewState"] = "candidate"
        with self.assertRaisesRegex(ValueError, "unapproved-image"):
            author.author_candidate(self.provider, **dict(self.args, approved_images=[]))

    def test_uncited_and_unbound_provenance_rejected(self):
        self.output["citations"] = ["made_up"]
        with self.assertRaisesRegex(ValueError, "uncited-source"):
            author.author_candidate(self.provider, **self.args)
        self.output["citations"] = ["source_test"]
        self.output["manifest"]["provenance"]["contentHash"] = "sha256:" + "0" * 64
        with self.assertRaisesRegex(ValueError, "provenance-binding"):
            author.author_candidate(self.provider, **self.args)

    def test_budget_is_durable_and_retained_on_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "budget.sqlite"
            ledger = author.SpendLedger(path, "run_test", "0.1")
            ledger.reserve("0.08")
            ledger.close()
            ledger = author.SpendLedger(path, "run_test", "0.1")
            try:
                with self.assertRaisesRegex(ValueError, "spending-cap"):
                    ledger.reserve("0.03")
                ledger.reserve("0.02")
                with self.assertRaisesRegex(ValueError, "spending-cap"):
                    author.author_candidate(self.provider, **dict(self.args, reserve_budget=ledger.reserve))
                self.assertEqual(self.calls, [])
            finally:
                ledger.close()

    def test_endpoint_cannot_downgrade_or_embed_credentials(self):
        for endpoint in ("http://api.example/chat", "https://name:pass@api.example/chat", "https://other.example/chat", "https://api.example/chat?key=value"):
            with self.assertRaises(ValueError):
                author.JSONChatProvider(endpoint, "model", "WAZI_MODEL_CREDENTIAL", "api.example")


if __name__ == "__main__":
    unittest.main()
