#!/usr/bin/env python3
"""Fail-closed source/application and image scans with redacted evidence."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


def secret_identity(target, finding):
    return (target, finding.get("RuleID"), finding.get("Category"),
            finding.get("Severity"), finding.get("StartLine"), finding.get("EndLine"))


def approve_shared_key(report, source_root, policy, now=None):
    """Bind the one reviewed exception to an unchanged file and exact finding."""
    now = now or datetime.now(timezone.utc)
    records = policy.get("exceptions")
    if not isinstance(records, list) or len(records) != 1:
        raise ValueError("Exactly one shared-key exception record is required")
    record = records[0]
    if not isinstance(record, dict) or record.get("id") != "shared-recording-signing-key":
        raise ValueError("Unexpected scan exception")
    if record.get("status") != "approved" or not all(
        isinstance(record.get(key), str) and record[key].strip()
        for key in ("owner", "approved_by", "reason", "approval_url", "expires")
    ):
        raise ValueError("Shared-key exception lacks approval/owner/reason/evidence/expiry")
    if record["approval_url"] != "https://github.com/kerberos-io/agent/pull/350":
        raise ValueError("Shared-key exception must reference its reviewed PR record")
    expires = datetime.fromisoformat(record["expires"].replace("Z", "+00:00"))
    if expires.tzinfo is None or expires <= now:
        raise ValueError("Shared-key exception expired or lacks timezone")
    if (record.get("target") != "machinery/src/config/main.go"
            or record.get("rule_id") != "private-key"
            or record.get("category") != "AsymmetricPrivateKey"
            or record.get("severity") != "HIGH"
            or type(record.get("start_line")) is not int
            or record["start_line"] < 1
            or record.get("end_line") != record["start_line"]):
        raise ValueError("Shared-key exception has an unexpected finding scope")
    root = Path(source_root).resolve()
    path = root / record["target"]
    if path.resolve() != path or hashlib.sha256(path.read_bytes()).hexdigest() != record.get("file_sha256"):
        raise ValueError("Shared-key source file changed or is a symlink; renewed review required")
    expected = (record["target"], record["rule_id"], record["category"],
                record["severity"], record["start_line"], record["end_line"])
    matches = [
        finding for result in report["Results"]
        for finding in result.get("Secrets", [])
        if secret_identity(result.get("Target"), finding) == expected
    ]
    if len(matches) != 1:
        raise ValueError("Shared-key exception must match exactly one existing source finding")
    return [dict(record)]


def check_report(report, image=False, source=False, approved_exceptions=()):
    if report.get("SchemaVersion") != 2 or not isinstance(report.get("Results"), list):
        raise ValueError("Missing or unsupported Trivy report")
    failures = []
    approved_secrets = {
        (record["target"], record["rule_id"], record["category"], record["severity"],
         record["start_line"], record["end_line"])
        for record in approved_exceptions
    } if source and not image else set()
    for result in report["Results"]:
        if result.get("Errors"):
            failures.append(f"Scan errors in {result.get('Target')}: {result['Errors']}")
        if result.get("MisconfSummary", {}).get("Exceptions", 0):
            failures.append(f"Unapproved misconfiguration exceptions in {result.get('Target')}")
        for kind in ("Vulnerabilities", "Secrets", "Misconfigurations"):
            for finding in result.get(kind, []):
                if kind == "Misconfigurations" and finding.get("Status") == "PASS":
                    continue
                if kind == "Misconfigurations" and finding.get("Status") != "FAIL":
                    failures.append(f"Unexpected/suppressed misconfiguration status: {finding.get('Status')}")
                severity = finding.get("Severity", "UNKNOWN")
                if kind == "Secrets" and secret_identity(result.get("Target"), finding) in approved_secrets:
                    continue
                if severity not in ("LOW", "MEDIUM"):
                    failures.append(f"{result.get('Target')}: {kind}: "
                                    f"{finding.get('VulnerabilityID', finding.get('ID', finding.get('RuleID')))} "
                                    f"{severity}")
    if image:
        # A scratch image without package metadata must not yield a false clean OS scan.
        if not report.get("Metadata", {}).get("OS", {}).get("Family"):
            failures.append("No OS detected: preserve runtime OS/package metadata")
        if not any(r.get("Class") == "os-pkgs" and r.get("Packages")
                   for r in report["Results"]):
            failures.append("No OS package inventory detected")
        if not any(r.get("Type") == "gobinary" and r.get("Packages")
                   for r in report["Results"]):
            failures.append("No Go application package inventory detected")
    if source and not any(
        r.get("Type") == "yarn" and r.get("Target", "").replace("\\", "/").endswith("ui/yarn.lock")
        and r.get("Packages") for r in report["Results"]
    ):
        failures.append("Missing production UI yarn.lock package inventory")
    return failures


def redact_report(value):
    """Keep findings actionable without persisting matches, excerpts or image env/history."""
    if isinstance(value, list):
        return [redact_report(item) for item in value]
    if not isinstance(value, dict):
        return value
    result = {}
    for key, child in value.items():
        if key.lower() in {"imageconfig", "buildinfo"}:
            continue
        if key.lower() in {
            "match", "code", "lines", "source", "sourcecode", "snippet", "context",
            "content",
        }:
            result[key] = "[REDACTED]"
        elif key == "Errors":
            result[key] = ["[REDACTED scanner error]"] if child else []
        elif key in ("Secrets", "Misconfigurations"):
            allowed = {
                "ID", "AVDID", "RuleID", "Type", "Category", "Title", "Severity",
                "Status", "StartLine", "EndLine", "References",
            }
            result[key] = [
                {field: finding[field] for field in allowed if field in finding}
                | {"SourceExcerpt": "[REDACTED]"}
                for finding in child
            ]
        else:
            result[key] = redact_report(child)
    return result


def scan(target, output, platform=None):
    output.mkdir(parents=True, exist_ok=True)
    config = output / "scanner.yaml"
    ignore = output / "empty.ignore"
    secret_config = output / "secret.yaml"
    config.write_text("{}\n")
    secret_config.write_text("{}\n")
    ignore.write_text("")
    # Do not consume repository trivy.yaml/.trivyignore or TRIVY_* overrides.
    env = {k: v for k, v in os.environ.items() if not k.startswith("TRIVY_")}
    command = [
        "trivy", "image" if platform else "fs", "--config", str(config),
        "--cache-dir", ".ci-tools/trivy-cache",
        "--ignorefile", str(ignore), "--secret-config", str(secret_config),
        "--format", "json", "--exit-code", "0",
        "--scanners", "vuln,secret,misconfig",
        "--severity", "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL", "--timeout", "20m",
        "--list-all-pkgs", "--detection-priority", "comprehensive",
    ]
    if platform:
        command += ["--platform", platform, "--pkg-types", "os,library",
                    "--image-config-scanners", "secret,misconfig"]
    else:
        # Include development/build dependencies too: none are silently exempted.
        # Trivy marks Dev packages using the adjacent package.json and yarn.lock.
        command += ["--pkg-types", "library", "--include-dev-deps"]
    command.append(target)
    # Never write raw scanner output, even on scanner failure/interruption. stderr
    # may contain source excerpts too; retain only a safe exit-status diagnostic.
    completed = subprocess.run(command, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, text=True, check=False)
    (output / "scanner.log").write_text(
        f"Trivy exit status: {completed.returncode}\n"
        "Raw scanner stdout/stderr omitted; scan.json is redacted before persistence.\n")
    if completed.returncode:
        raise ValueError(f"Trivy failed with exit status {completed.returncode}; raw diagnostics withheld")
    try:
        report = redact_report(json.loads(completed.stdout))
    except ValueError:
        raise ValueError("Invalid scanner JSON; raw output withheld") from None
    (output / "scan.json").write_text(json.dumps(report, indent=2) + "\n")
    approved = []
    exception_failures = []
    if not platform:
        try:
            policy = json.loads(Path(__file__).with_name("image-scan-exceptions.json").read_text())
            (output / "exception-policy.json").write_text(json.dumps(policy, indent=2) + "\n")
            approved = approve_shared_key(report, target, policy)
        except (OSError, ValueError, TypeError, KeyError) as error:
            exception_failures.append(f"Shared-key exception invalid: {error}")
    (output / "exception-decisions.json").write_text(json.dumps({
        "approved_exceptions": approved, "failures": exception_failures,
    }, indent=2) + "\n")
    failures = exception_failures + check_report(
        report, image=bool(platform), source=not platform, approved_exceptions=approved)
    if not platform:
        inventory = [{
            "target": result.get("Target"), "type": result.get("Type"),
            "packages": [{"name": pkg.get("Name"), "version": pkg.get("Version"),
                          "scope": "development" if pkg.get("Dev") else "production-or-unclassified"}
                         for pkg in result.get("Packages", [])],
        } for result in report["Results"] if result.get("Packages")]
        (output / "application-inventory.json").write_text(json.dumps({
            "policy": "All production and development/build dependencies are gated; "
                      "only the reviewed, exact shared-key secret exception may apply.",
            "inventories": inventory,
        }, indent=2) + "\n")
    converted = subprocess.run([
        "trivy", "convert", "--cache-dir", ".ci-tools/trivy-cache",
        "--format", "spdx-json", "--output", str(output / "sbom.spdx.json"),
        str(output / "scan.json"),
    ], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if converted.returncode:
        raise ValueError(f"SPDX conversion failed with exit status {converted.returncode}")
    sbom = json.loads((output / "sbom.spdx.json").read_text())
    if not sbom.get("spdxVersion") or not sbom.get("packages"):
        failures.append("Missing/empty SPDX SBOM")
    return failures


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("target", help="Repository root, or final image reference with --platform")
    parser.add_argument("output", type=Path, help="Directory for redacted reports and SPDX SBOM")
    parser.add_argument("--platform", help="Image platform, e.g. linux/amd64; omit for source/UI scanning")
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    approved = []
    try:
        failures = scan(args.target, args.output, args.platform)
        approved = json.loads((args.output / "exception-decisions.json").read_text())["approved_exceptions"]
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        failures = [f"Scanner/evidence error: {error}"]
    (args.output / "policy.json").write_text(json.dumps({
        "target": args.target, "platform": args.platform, "failures": failures,
        "approved_exceptions": approved, "passed": not failures,
    }, indent=2) + "\n")
    for failure in failures:
        print(failure, file=sys.stderr)
    return int(bool(failures))


if __name__ == "__main__":
    sys.exit(main())
