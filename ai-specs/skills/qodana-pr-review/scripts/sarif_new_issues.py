#!/usr/bin/env python3
"""Report Qodana results whose baselineState is "new"; optionally baseline some.

Usage:
  sarif_new_issues.py <qodana.sarif.json>
      Print new issues, numbered (#N), grouped by Qodana severity.
  sarif_new_issues.py --accept 1,3 <qodana.sarif.json> <baseline.sarif.json>
      Append the selected new results (report numbering) to the baseline so
      the next PR-mode scan treats them as unchanged.
"""
import collections
import copy
import json
import sys

# Qodana severity scale (properties.qodanaSeverity), most severe first.
ORDER = ["Critical", "High", "Moderate", "Low", "Info"]
LEVEL_FALLBACK = {"error": "High", "warning": "Moderate", "note": "Low", "none": "Info"}


def new_issues(sarif):
    issues = []
    for run in sarif.get("runs", []):
        for res in run.get("results", []):
            props = res.get("properties", {})
            # SARIF 2.1.0 puts baselineState at the top level (Qodana does too).
            state = res.get("baselineState") or props.get("baselineState")
            if state != "new":
                continue
            loc = (res.get("locations") or [{}])[0].get("physicalLocation", {})
            issues.append({
                "raw": res,
                "severity": props.get("qodanaSeverity") or LEVEL_FALLBACK.get(res.get("level"), "Info"),
                "rule": res.get("ruleId", "?"),
                "file": loc.get("artifactLocation", {}).get("uri", "?"),
                "line": loc.get("region", {}).get("startLine", 0),
                "msg": res.get("message", {}).get("text", "").strip(),
            })
    rank = {s: i for i, s in enumerate(ORDER)}
    issues.sort(key=lambda i: (rank.get(i["severity"], len(ORDER)), i["file"], i["line"]))
    return issues


def render(issues):
    lines = [f"=== Qodana PR review: {len(issues)} new issue(s) vs baseline ==="]
    counts = collections.Counter(i["severity"] for i in issues)
    for sev in ORDER + sorted(set(counts) - set(ORDER)):
        if counts.get(sev):
            lines.append(f"  {sev}: {counts[sev]}")
    for n, i in enumerate(issues, 1):
        lines.append(f"  #{n} [{i['severity']}] {i['rule']}  {i['file']}:{i['line']}")
        if i["msg"]:
            lines.append(f"          {i['msg']}")
    if not issues:
        lines.append("  No new issues. Safe to open the PR.")
    return "\n".join(lines)


def accept(baseline, current, numbers):
    """Append selected new results (1-based report numbers) to the baseline."""
    issues = new_issues(current)
    bad = [n for n in numbers if not 1 <= n <= len(issues)]
    if bad:
        raise ValueError(f"issue numbers out of range 1..{len(issues)}: {bad}")
    target = baseline.setdefault("runs", [{}])[0].setdefault("results", [])
    for n in sorted(set(numbers)):
        res = copy.deepcopy(issues[n - 1]["raw"])
        res.pop("baselineState", None)
        res.get("properties", {}).pop("baselineState", None)
        target.append(res)
    return len(set(numbers))


def _load(path):
    with open(path, encoding="utf-8") as fh:
        return json.load(fh)


if __name__ == "__main__":
    args = sys.argv[1:]
    if args and args[0] == "--accept":
        nums = [int(x) for x in args[1].split(",") if x.strip()]
        current_path, baseline_path = args[2], args[3]
        baseline = _load(baseline_path)
        added = accept(baseline, _load(current_path), nums)
        with open(baseline_path, "w", encoding="utf-8") as fh:
            json.dump(baseline, fh, indent=2)
        print(f"Added {added} issue(s) to {baseline_path}")
    else:
        print("\n" + render(new_issues(_load(args[0]))))
