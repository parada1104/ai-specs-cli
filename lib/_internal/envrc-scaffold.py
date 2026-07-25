#!/usr/bin/env python3
"""Compatibility shim — prefer env_scaffold.py."""
from __future__ import annotations

import importlib.util
import sys
from pathlib import Path


def _load_env_scaffold():
    path = Path(__file__).with_name("env_scaffold.py")
    spec = importlib.util.spec_from_file_location("env_scaffold", path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load env_scaffold.py at {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


_env = _load_env_scaffold()

ENV_REFERENCE_RE = _env.ENV_REFERENCE_RE
ENV_VAR_HELP = _env.ENV_VAR_HELP
MANAGED_START = _env.MANAGED_START
MANAGED_END = _env.MANAGED_END
collect_env_vars = _env.collect_env_vars
generate_env_example = _env.generate_env_example
generate_envrc_example = _env.generate_envrc_example
prompt_env_vars = _env.prompt_env_vars
write_env = _env.write_env
load_harness_env = _env.load_harness_env
write_envrc = _env.write_envrc
ensure_root_envrc = _env.ensure_root_envrc
has_managed_block = _env.has_managed_block
migrate_legacy_envrc = _env.migrate_legacy_envrc
direnv_allow = _env.direnv_allow
offer_harness_env = _env.offer_harness_env
main = _env.main

if __name__ == "__main__":
    raise SystemExit(main())
