#!/usr/bin/env python3
"""Release-only policy gate. Approval records require a reviewed repository change."""

from datetime import datetime, timezone
import json
from pathlib import Path
import sys


def check_policy(policy, now=None):
    now = now or datetime.now(timezone.utc)
    records = policy.get("required_exceptions")
    if not isinstance(records, list) or not records:
        return ["Required exception records are missing"]
    if {record.get("id") for record in records} != {"remote-terminal-pty"}:
        return ["Required remote-terminal-pty exception record is missing or unexpected"]
    failures = []
    for record in records:
        name = record["id"]
        if record.get("status") != "approved":
            failures.append(f"{name}: required exception is PENDING, not approved")
            continue
        if not all(isinstance(record.get(key), str) and record[key].strip()
                   for key in ("owner", "approved_by", "reason", "approval_url", "expires")):
            failures.append(f"{name}: missing owner/approver/reason/approval evidence/expiry")
            continue
        if not record["approval_url"].startswith("https://github.com/kerberos-io/agent/"):
            failures.append(f"{name}: approval must reference this repository's reviewed record")
        try:
            expires = datetime.fromisoformat(record["expires"].replace("Z", "+00:00"))
            if expires.tzinfo is None or expires <= now:
                failures.append(f"{name}: approval expired or lacks timezone")
        except ValueError:
            failures.append(f"{name}: invalid approval expiry")
    return failures


if __name__ == "__main__":
    try:
        failures = check_policy(json.loads(Path(sys.argv[1]).read_text()))
    except (OSError, ValueError, TypeError, KeyError) as error:
        failures = [f"Invalid release policy: {error}"]
    for failure in failures:
        print(failure, file=sys.stderr)
    sys.exit(bool(failures))
