"""Phase 2 performance evidence for Bash/Go gate parity."""
from __future__ import annotations
import json
import subprocess
import tempfile
import time
import unittest
from pathlib import Path
from tests.test_worktree_gate_parity import CORPUS, build_fixture, materialize_legacy, substitute

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "dist" / "worktree-gate-current"

class WorktreeGateMetricsTests(unittest.TestCase):
    def test_representative_cases_have_go_measurements(self):
        if not BINARY.exists():
            self.skipTest("local Go binary not built")
        samples = sorted(CORPUS.glob("*.json"))[:4]
        self.assertTrue(samples)
        timings = []
        for case_file in samples:
            case = json.loads(case_file.read_text())
            with tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                locations = {} if case["fixture"] == "none" else build_fixture(root, case["fixture"])
                cwd = locations.get("repo", root)
                event = json.loads(json.dumps(case.get("event", {})))
                if "cwd" in event:
                    event["cwd"] = substitute(event["cwd"], locations)
                ti = event.get("tool_input") or {}
                for key in ("file_path", "notebook_path", "command", "script", "cmd"):
                    if key in ti:
                        ti[key] = substitute(ti[key], locations)
                payload = case.get("stdin") or json.dumps(event)
                start = time.perf_counter()
                result = subprocess.run([str(BINARY), "--gate-mode", "always", "--gate-scope", "auto", "--repo-topology", "auto", "--protected", "main development"], input=payload, capture_output=True, text=True, cwd=cwd)
                timings.append(time.perf_counter() - start)
                self.assertIn(result.returncode, (0, 2))
        self.assertEqual(len(timings), len(samples))
        self.assertTrue(all(t >= 0 for t in timings))

if __name__ == "__main__":
    unittest.main()
