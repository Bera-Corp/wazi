#!/usr/bin/env python3
"""Independently validate Go-produced host-profile JSON against the
canonical schema.

The Go intake tests assert schema validity with Go-side stand-ins.
This gate closes the loop in CI: the Go round-trip test writes its
actual intake -> storage -> GET listing to a file when
WAZI_ROUNDTRIP_OUT is set, and this script validates every profile
in that listing against schemas/host-profile.schema.json (resolving
the common.schema.json $ref), exactly as the Phase 0 validator
would.

Usage: validate_go_profile.py <listing.json>
Exit 0 = every profile valid; 1 = invalid or unreadable.
"""

import json
import sys
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012


def main(argv):
    if len(argv) != 2:
        print("usage: validate_go_profile.py <listing.json>")
        return 1
    root = Path(__file__).resolve().parent.parent
    schemas = root / "schemas"
    host_schema = json.loads((schemas / "host-profile.schema.json").read_text(encoding="utf-8"))
    common_schema = json.loads((schemas / "common.schema.json").read_text(encoding="utf-8"))
    registry = Registry().with_resource(
        "common.schema.json", Resource.from_contents(common_schema, default_specification=DRAFT202012)
    )
    # The host schema $refs "common.schema.json#/$defs/redaction";
    # register it under the exact relative name used by the $ref.
    validator = Draft202012Validator(host_schema, registry=registry, format_checker=FormatChecker())

    listing_path = Path(argv[1])
    try:
        listing = json.loads(listing_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        print(f"cannot read Go round-trip output {listing_path}: {exc}")
        return 1
    if not isinstance(listing, dict) or not listing:
        print("Go round-trip output contains no profiles")
        return 1
    failures = 0
    for host_id, profile in sorted(listing.items()):
        errors = sorted(validator.iter_errors(profile), key=lambda e: list(e.path))
        for error in errors:
            failures += 1
            print(f"{host_id}: canonical schema violation at {list(error.path)}: {error.message}")
    if failures:
        print(f"FAILED: {failures} canonical schema violation(s) in Go round-trip output")
        return 1
    print(f"OK: {len(listing)} Go round-trip profile(s) valid against the canonical schema")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
