"""Contract for the qodana-pr-review skill's SARIF new-issue reporter."""
import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "ai-specs/skills/qodana-pr-review/scripts/sarif_new_issues.py"


def load():
    spec = importlib.util.spec_from_file_location("sarif_new_issues", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def result(state, severity, uri, line, rule="GoUnhandledErrorResult", in_props=False):
    res = {
        "ruleId": rule,
        "level": "warning",
        "message": {"text": "Unhandled error"},
        "locations": [{"physicalLocation": {
            "artifactLocation": {"uri": uri},
            "region": {"startLine": line},
        }}],
        "properties": {"qodanaSeverity": severity},
    }
    if in_props:
        res["properties"]["baselineState"] = state
    else:
        res["baselineState"] = state  # SARIF 2.1.0 / Qodana: top-level field
    return res


class NewIssuesTest(unittest.TestCase):
    def test_only_new_results_are_reported_sorted_by_severity(self):
        mod = load()
        sarif = {"runs": [{"results": [
            result("unchanged", "High", "a.go", 1),
            result("new", "Moderate", "b.go", 9),
            result("new", "High", "c.go", 3),
            result("absent", "High", "d.go", 4),
        ]}]}
        issues = mod.new_issues(sarif)
        self.assertEqual([(i["severity"], i["file"], i["line"]) for i in issues],
                         [("High", "c.go", 3), ("Moderate", "b.go", 9)])

    def test_properties_baseline_state_is_accepted_as_fallback(self):
        mod = load()
        sarif = {"runs": [{"results": [result("new", "High", "x.go", 2, in_props=True)]}]}
        self.assertEqual(len(mod.new_issues(sarif)), 1)

    def test_report_groups_counts_by_severity(self):
        mod = load()
        sarif = {"runs": [{"results": [
            result("new", "High", "c.go", 3),
            result("new", "High", "c.go", 7),
            result("new", "Moderate", "b.go", 9),
        ]}]}
        text = mod.render(mod.new_issues(sarif))
        self.assertIn("3 new issue(s)", text)
        self.assertIn("High: 2", text)
        self.assertIn("Moderate: 1", text)
        self.assertIn("c.go:7", text)

    def test_empty_report_says_safe(self):
        mod = load()
        self.assertIn("No new issues", mod.render([]))

    def test_report_numbers_issues_for_selection(self):
        mod = load()
        sarif = {"runs": [{"results": [result("new", "High", "c.go", 3)]}]}
        self.assertIn("#1 [High]", mod.render(mod.new_issues(sarif)))

    def test_accept_appends_only_selected_new_results_to_baseline(self):
        mod = load()
        baseline = {"runs": [{"results": [result(None, "High", "a.go", 1)]}]}
        del baseline["runs"][0]["results"][0]["baselineState"]
        current = {"runs": [{"results": [
            result("unchanged", "High", "a.go", 1),
            result("new", "Moderate", "b.go", 9),
            result("new", "High", "c.go", 3),
        ]}]}
        # Report order: #1 = High c.go:3, #2 = Moderate b.go:9
        added = mod.accept(baseline, current, [2])
        self.assertEqual(added, 1)
        uris = [r["locations"][0]["physicalLocation"]["artifactLocation"]["uri"]
                for r in baseline["runs"][0]["results"]]
        self.assertEqual(uris, ["a.go", "b.go"])
        self.assertNotIn("baselineState", baseline["runs"][0]["results"][1])

    def test_accept_rejects_out_of_range_selection(self):
        mod = load()
        current = {"runs": [{"results": [result("new", "High", "c.go", 3)]}]}
        with self.assertRaises(ValueError):
            mod.accept({"runs": [{"results": []}]}, current, [2])


if __name__ == "__main__":
    unittest.main()
