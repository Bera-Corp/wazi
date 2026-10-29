#!/usr/bin/env python3
"""Focused tests for the dedicated secret scanner."""
import importlib.util
import sys
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("secret_scan", HERE / "secret_scan.py")
scanner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scanner)


def main():
    results = []
    synthetic = "api_key = " + "xK9" * 8  # synthetic credential-shaped value
    found = scanner.scan_text(synthetic)
    results.append(bool(found))
    print(("PASS" if found else "FAIL") + " scanner flags synthetic assigned credential (value suppressed)")

    clean = 'secretRef = "secret://deployment/app-secret"\nname = "wazi"\n'
    found = scanner.scan_text(clean)
    results.append(not found)
    print(("PASS" if not found else "FAIL") + " scanner allows secret:// references")

    commented = 'api_key = "xK9xK9xK9xK9xK9"  # see secret://deployment/app-secret\n'
    found = scanner.scan_text(commented)
    results.append(bool(found))
    print(("PASS" if found else "FAIL") + " reference comment on the same line does not suppress an assignment")

    json_line = '{"api_key": "xK9xK9xK9xK9xK9"}\n'
    found = scanner.scan_text(json_line)
    results.append(bool(found))
    print(("PASS" if found else "FAIL") + " quoted JSON assignment keys are scanned")

    with tempfile.TemporaryDirectory() as tmp:
        bad = Path(tmp) / "bad.txt"
        bad.write_text(synthetic, encoding="utf-8")
        rc = scanner.main(["secret_scan.py", tmp])
        results.append(rc == 1)
        print(("PASS" if rc == 1 else "FAIL") + " scanner exits 1 on a dirty tree")

    with tempfile.TemporaryDirectory() as tmp:
        nested = Path(tmp) / "nested"
        nested.mkdir()
        (nested / "validate.py").write_text(synthetic, encoding="utf-8")
        rc = scanner.main(["secret_scan.py", tmp])
        results.append(rc == 1)
        print(("PASS" if rc == 1 else "FAIL") + " basename validate.py outside scripts/ is still scanned")

    with tempfile.TemporaryDirectory() as tmp:
        scripts_dir = Path(tmp) / "scripts"
        scripts_dir.mkdir()
        (scripts_dir / "validate.py").write_text(synthetic, encoding="utf-8")
        rc = scanner.main(["secret_scan.py", tmp])
        results.append(rc == 0)
        print(("PASS" if rc == 0 else "FAIL") + " reviewed scripts/validate.py path is excluded")

    print("ALL PASS" if all(results) else "SOME FAILED")
    return 0 if all(results) else 1


if __name__ == "__main__":
    sys.exit(main())
