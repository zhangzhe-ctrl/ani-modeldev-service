#!/usr/bin/env python3
"""Extract independently pinned CPU-P01 vectors on the Fedora task checkout.

This intentionally reads handwritten Go raw-string literals, not outputs from
the Go canonicalization implementation. It does not resolve environment values.
"""

import argparse
import hashlib
import json
from pathlib import Path
import re


INTENT_SHA256 = "152ed6b8e0a392be5697ee138e5a22be4183139c2954f009d6d85483cdf4a01a"
SNAPSHOT_SHA256 = "972dee14e65202d5d4da7da199cf5f37139b535701a0cb1b3de4d8ef8a5170b9"


def literal(source: Path, function: str, variable: str, byte_slice: bool) -> bytes:
    text = source.read_text(encoding="utf-8")
    declaration = re.search(r"^func " + re.escape(function) + r"\(", text, re.MULTILINE)
    if declaration is None:
        raise ValueError(f"missing normative test {function}")
    body = text[declaration.end():]
    next_function = re.search(r"^func ", body, re.MULTILINE)
    if next_function is not None:
        body = body[:next_function.start()]
    value_pattern = r"\[\]byte\(`([^`]*)`\)" if byte_slice else r"`([^`]*)`"
    values = re.findall(r"\b" + re.escape(variable) + r"\s*:=\s*" + value_pattern, body)
    if len(values) != 1:
        raise ValueError(f"expected one handwritten {function}/{variable} literal")
    content = values[0].encode("utf-8")
    if content.startswith(b"\xef\xbb\xbf") or content.endswith(b"\n"):
        raise ValueError(f"normative {function}/{variable} contains BOM or trailing newline")
    if not isinstance(json.loads(content), dict):
        raise ValueError(f"normative {function}/{variable} is not a JSON object")
    return content


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="compare only; never write files")
    arguments = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    contract = root / "contract" / "cpup01"
    intent_test = contract / "intent_test.go"
    snapshot_test = contract / "snapshot_test.go"
    outputs = {
        "intent-input-v1.json": literal(intent_test, "TestIntentCanonicalizesUserRequestWithoutResolvingDefaults", "raw", True),
        "intent-canonical-v1.json": literal(intent_test, "TestIntentCanonicalizesUserRequestWithoutResolvingDefaults", "want", False),
        "snapshot-v1.json": literal(snapshot_test, "TestSnapshotCanonicalizesCompleteFrozenConfiguration", "want", False),
    }
    for filename, expected in {
        "intent-canonical-v1.json": INTENT_SHA256,
        "snapshot-v1.json": SNAPSHOT_SHA256,
    }.items():
        if hashlib.sha256(outputs[filename]).hexdigest() != expected:
            raise ValueError(f"independent SHA256 mismatch: {filename}")
    destination = contract / "conformance"
    if not arguments.check:
        destination.mkdir(parents=True, exist_ok=True)
    for filename, content in outputs.items():
        target = destination / filename
        if arguments.check:
            if not target.is_file() or target.read_bytes() != content:
                raise ValueError(f"generated fixture differs or is missing: {filename}")
        else:
            target.write_bytes(content)
        print(f"{hashlib.sha256(content).hexdigest()}  {filename}")


if __name__ == "__main__":
    main()
