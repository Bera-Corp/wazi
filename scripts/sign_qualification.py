"""Sign one runner receipt with an operator-managed qualification-only key."""
import argparse
import datetime as dt
import json
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("input", "output", "key", "signer-id", "policy", "recordtool", "cosign"):
        parser.add_argument("--" + name, required=True)
    args = parser.parse_args()
    record = json.loads(Path(args.input).read_text(encoding="utf-8"))
    if "signature" in record or "qualificationId" not in record:
        raise ValueError("unsigned-qualification-required")
    # Reserve the destination before invoking the signer. A failed attempt is
    # explicit evidence and cannot silently overwrite another signed record.
    with Path(args.output).open("x", encoding="utf-8") as output:
        with tempfile.TemporaryDirectory(prefix="wazi-qualification-sign-") as temporary:
            directory = Path(temporary)
            payload = directory / "payload.txt"
            signature = directory / "signature.txt"
            digest = subprocess.check_output([args.recordtool, "digest", args.input], text=True).strip()
            payload.write_text(digest, encoding="ascii")
            subprocess.run([args.cosign, "sign-blob", "--key", args.key, "--new-bundle-format=false", "--use-signing-config=false", "--tlog-upload=false", "--output-signature", str(signature), str(payload)], check=True)
            record["signature"] = {"algorithm":"sigstore-cosign", "signedBy":args.signer_id,
                                   "signedAt":dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds").replace("+00:00","Z"),
                                   "payloadDigest":digest, "value":signature.read_text(encoding="ascii").strip()}
            signed = directory / "signed.json"
            signed.write_text(json.dumps(record, sort_keys=True, separators=(",", ":")), encoding="utf-8")
            subprocess.run([args.recordtool, "verify", str(signed), args.policy], check=True)
            output.write(signed.read_text(encoding="utf-8"))


if __name__ == "__main__":
    main()
