import json
from pathlib import Path
import unittest

import context_engine as engine
import prepare_phase2_release as release


class PackageReleaseShapeTests(unittest.TestCase):
    def test_all_unsigned_packages_validate_before_signing(self):
        root = Path(__file__).resolve().parents[1]
        for profile in release.SHAPES:
            metadata = json.loads((root/'releases/phase2'/profile/'image-export.json').read_text())
            records = release.unsigned_records(profile, metadata, '2026-10-31T12:00:00Z')
            for name, record in records.items():
                schema = 'app-manifest.schema.json' if name == 'manifest' else 'procedure.schema.json'
                self.assertEqual(engine._record_schema_errors(schema, dict(record, reviewState='candidate')), [], (profile, name))
            self.assertFalse(records['manifest']['release']['rollback']['supported'])
            self.assertNotIn('rollback', records)
            self.assertEqual(records['procedure-restore']['requiresCheckpoint'], True)


if __name__ == '__main__':
    unittest.main()
