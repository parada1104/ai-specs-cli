#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

python3 -m py_compile lib/_internal/*.py tests/*.py
bash -n lib/*.sh bin/ai-specs tests/*.sh
./tests/run.sh
