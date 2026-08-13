// child_process_double.mjs — recording spawnSync test double.
//
// Substituted for `node:child_process`'s spawnSync by the process-boundary
// harness (opencode_process_boundary.mjs). The double never spawns anything:
// it records the exact invocation the generated adapter makes — executable
// path, parsed input JSON, options cwd — and returns a controllable result
// so the test can observe real process-boundary behavior deterministically.
//
// Control comes from `globalThis.__WT_TEST_DOUBLE__`, set by the harness
// before the generated module is imported:
//   { status: <exit code>, stderr: <text>, error: <text>, throw: <text> }
// - `error` returns a result carrying `.error` (spawn failure; status null)
// - `throw` makes spawnSync itself throw (child-process exception)
// Records accumulate on `globalThis.__WT_RECORDS__` for the harness to emit.

export function spawnSync(command, options) {
  const cfg = globalThis.__WT_TEST_DOUBLE__ ?? {};
  if (cfg.throw) {
    throw new Error(cfg.throw);
  }
  let input = null;
  try {
    input = JSON.parse(String(options?.input ?? ""));
  } catch {
    input = null;
  }
  (globalThis.__WT_RECORDS__ ??= []).push({
    command,
    input,
    optionsCwd: options?.cwd ?? null,
  });
  const res = { status: null, stderr: cfg.stderr ?? "", stdout: cfg.stdout ?? "" };
  if (cfg.error) {
    res.error = new Error(cfg.error);
  } else {
    res.status = cfg.status ?? 0;
  }
  return res;
}
