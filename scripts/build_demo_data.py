#!/usr/bin/env python3
"""Generate demo/data.js from the Phase 2 retrieval pipeline.

The demo never hand-writes its denials or rankings: every scenario in
the packet is produced by scripts/context_engine.py running the real
hard filters and scored ranking over the canonical examples.

    .venv/Scripts/python.exe scripts/build_demo_data.py
"""

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import context_engine as ce  # noqa: E402


def main():
    packet = ce.build_demo_packet()
    out = ce.ROOT / "demo" / "data.js"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("window.WAZI_DEMO = " + json.dumps(packet, indent=1) + ";\n",
                   encoding="utf-8")
    decisions = [s["result"]["decision"] for s in packet["scenarios"]]
    print(f"wrote {out} scenarios={decisions} "
          f"endToEnd={packet['metrics']['endToEndAccuracy']} "
          f"falseAccept={packet['metrics']['falseAcceptRate']} "
          f"hardEngine={packet['metrics']['hardEngineDecisionAccuracy']}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
