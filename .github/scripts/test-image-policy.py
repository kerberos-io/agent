#!/usr/bin/env python3
"""Focused release guard tests; no registry, signing, or scanner network access."""

from datetime import datetime, timezone
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import subprocess


ROOT = Path(__file__).parent


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


scanner = load("image-policy")
release = load("image-release-policy")
evidence = load("image-evidence")


class ScanPolicyTests(unittest.TestCase):
    def report(self, severity=None, kind="Vulnerabilities"):
        result = {"Class": "os-pkgs", "Packages": [{"Name": "libc6"}]}
        if severity:
            result[kind] = [{"Severity": severity, "Status": "FAIL", "ID": "fixture"}]
        return {"SchemaVersion": 2, "Metadata": {"OS": {"Family": "debian"}},
                "Results": [result, {"Class": "lang-pkgs", "Type": "gobinary",
                                     "Packages": [{"Name": "stdlib"}]}]}

    def test_low_and_medium_are_not_blocking(self):
        for severity in ("LOW", "MEDIUM"):
            self.assertEqual(scanner.check_report(self.report(severity), image=True), [])

    def test_high_critical_unknown_and_unrecognized_block(self):
        for kind in ("Vulnerabilities", "Secrets", "Misconfigurations"):
            for severity in ("HIGH", "CRITICAL", "UNKNOWN", "invalid"):
                with self.subTest(kind=kind, severity=severity):
                    self.assertTrue(scanner.check_report(self.report(severity, kind), image=True))

    def test_missing_inventory_blocks_false_clean_scratch_scan(self):
        report = self.report()
        report["Metadata"] = {}
        report["Results"] = []
        self.assertEqual(len(scanner.check_report(report, image=True)), 3)

    def test_empty_os_inventory_blocks(self):
        report = self.report()
        report["Results"][0]["Packages"] = []
        self.assertTrue(scanner.check_report(report, image=True))

    def test_report_errors_and_suppression_summary_block(self):
        report = self.report()
        report["Results"][0]["Errors"] = ["fixture scanner error"]
        report["Results"][0]["MisconfSummary"] = {"Exceptions": 1}
        self.assertEqual(len(scanner.check_report(report)), 2)

    def test_missing_or_unsupported_report_blocks(self):
        for report in ({}, {"SchemaVersion": 1, "Results": []}):
            with self.assertRaises(ValueError):
                scanner.check_report(report)

    def test_suppressed_misconfiguration_blocks(self):
        report = self.report("LOW", "Misconfigurations")
        report["Results"][0]["Misconfigurations"][0]["Status"] = "EXCEPTION"
        self.assertTrue(scanner.check_report(report))

    def test_scanner_errors_are_not_ignored(self):
        directory = unittest.mock.MagicMock()
        with patch.object(scanner.subprocess, "run",
                          side_effect=subprocess.CalledProcessError(1, ["trivy"])):
            with self.assertRaises(subprocess.CalledProcessError):
                scanner.scan("fixture", directory, "linux/amd64")

    def test_source_requires_ui_lockfile_inventory(self):
        report = self.report()
        self.assertTrue(scanner.check_report(report, source=True))
        report["Results"].append({"Type": "yarn", "Target": "ui/yarn.lock",
                                   "Packages": [{"Name": "react", "Version": "17.0.2"}]})
        self.assertEqual(scanner.check_report(report, source=True), [])
        report["Results"][-1]["Vulnerabilities"] = [{"Severity": "HIGH", "VulnerabilityID": "fixture"}]
        self.assertTrue(scanner.check_report(report, source=True))

    def test_secret_and_source_excerpts_are_removed_without_waiving_findings(self):
        marker = "sensitive-fixture-text-not-a-real-credential"
        report = self.report("HIGH", "Secrets")
        report["Metadata"]["ImageConfig"] = {"config": {"Env": [marker]}}
        report["Metadata"]["BuildInfo"] = {"attrs": {"secret": marker}}
        report["Results"][0]["Secrets"][0].update(
            Match=marker, Code={"Lines": [{"Content": marker}]}, Message=marker,
            UnexpectedFutureExcerptField=marker, RuleID="private-key-fixture")
        report["Results"][0]["Misconfigurations"] = [{
            "ID": "fixture", "Severity": "HIGH", "Status": "FAIL",
            "Message": marker, "CauseMetadata": {"Code": {"Lines": [{"Content": marker}]}}
        }]
        report["Results"][0]["Errors"] = [marker]
        redacted = scanner.redact_report(report)
        self.assertNotIn(marker, json.dumps(redacted))
        self.assertNotIn("ImageConfig", redacted["Metadata"])
        self.assertTrue(scanner.check_report(redacted, image=True))
        self.assertEqual(redacted["Results"][0]["Secrets"][0]["RuleID"], "private-key-fixture")

    def test_source_scan_captures_output_in_memory_and_generates_sbom(self):
        report = self.report()
        report["Results"].append({"Type": "yarn", "Target": "ui/yarn.lock",
                                   "Packages": [{"Name": "fixture", "Version": "1.0", "Dev": True}]})
        directory = unittest.mock.MagicMock()
        directory.__truediv__.return_value.read_text.return_value = json.dumps({
            "spdxVersion": "SPDX-2.3", "packages": [{"name": "fixture"}]})
        completed = subprocess.CompletedProcess(["trivy"], 0, stdout=json.dumps(report), stderr="")
        with patch.object(scanner.subprocess, "run", return_value=completed) as run:
            self.assertEqual(scanner.scan("fixture", directory), [])
        command = run.call_args_list[0].args[0]
        self.assertIn("--include-dev-deps", command)
        self.assertIn("vuln,secret,misconfig", command)
        self.assertNotIn("--output", command)
        self.assertEqual(run.call_args_list[0].kwargs["stdout"], subprocess.PIPE)
        self.assertEqual(run.call_args_list[1].args[0][1], "convert")

    def test_raw_scanner_error_diagnostics_are_not_persisted(self):
        directory = unittest.mock.MagicMock()
        marker = "sensitive-fixture-text-not-a-real-credential"
        completed = subprocess.CompletedProcess(["trivy"], 1, stdout=marker, stderr=marker)
        with patch.object(scanner.subprocess, "run", return_value=completed):
            with self.assertRaisesRegex(ValueError, "exit status 1"):
                scanner.scan("fixture", directory)
        self.assertNotIn(marker, str(directory.mock_calls))


class ReleasePolicyTests(unittest.TestCase):
    NOW = datetime(2026, 11, 4, tzinfo=timezone.utc)

    def test_checked_in_state_blocks_publication(self):
        state = json.loads((ROOT / "image-release-policy.json").read_text())
        self.assertTrue(release.check_policy(state, self.NOW))

    def test_missing_records_block(self):
        for state in ({}, {"required_exceptions": []},
                      {"required_exceptions": [{"id": "unrelated"}]}):
            self.assertTrue(release.check_policy(state, self.NOW))

    def test_expired_or_incomplete_approval_blocks(self):
        state = json.loads((ROOT / "image-release-policy.json").read_text())
        record = state["required_exceptions"][0]
        record["status"] = "approved"
        self.assertTrue(release.check_policy(state, self.NOW))
        record.update(owner="test-fixture", approved_by="test-fixture",
                      approval_url="https://github.com/kerberos-io/agent/issues/1",
                      expires="2020-01-01T00:00:00Z")
        self.assertTrue(release.check_policy(state, self.NOW))
        record["expires"] = "invalid"
        self.assertTrue(release.check_policy(state, self.NOW))
        record["expires"] = "2027-01-01"
        self.assertTrue(release.check_policy(state, self.NOW))


class ProvenanceTests(unittest.TestCase):
    def test_base_digest_retained(self):
        material = {"uri": "pkg:docker/debian@trixie", "digest": {"sha256": "a" * 64}}
        self.assertEqual(list(evidence.materials({"SLSA": {"materials": [material]}})), [material])

    def test_missing_digest_rejected(self):
        with self.assertRaises(ValueError):
            list(evidence.materials({"uri": "pkg:docker/debian@trixie"}))


if __name__ == "__main__":
    unittest.main()
