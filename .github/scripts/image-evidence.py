#!/usr/bin/env python3
"""Validate BuildKit provenance and retain resolved base-image materials."""

import json
from pathlib import Path
import re
import sys


def materials(value):
    if isinstance(value, dict):
        if "uri" in value and value["uri"].startswith(("pkg:docker/", "docker-image://")):
            digest = value.get("digest", {}).get("sha256", "")
            if not re.fullmatch(r"[a-f0-9]{64}", digest):
                raise ValueError(f"Missing base-image digest: {value['uri']}")
            yield value
        for child in value.values():
            yield from materials(child)
    elif isinstance(value, list):
        for child in value:
            yield from materials(child)


if __name__ == "__main__":
    directory = Path(sys.argv[1])
    provenance = json.loads((directory / "provenance.json").read_text())
    bases = list(materials(provenance))
    if not bases:
        raise SystemExit("Missing provenance/base-image materials")
    (directory / "base-image-digests.json").write_text(json.dumps(bases, indent=2) + "\n")
