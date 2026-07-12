#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
modules=()
for f in tests/evals/eval_*.py; do
  [ -f "$f" ] || continue
  modules+=("tests.evals.$(basename "$f" .py)")
done
if [ ${#modules[@]} -eq 0 ]; then
  echo "no eval_*.py modules found" >&2
  exit 1
fi
python3 -m unittest "${modules[@]}" "$@"
