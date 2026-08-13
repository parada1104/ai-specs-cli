#!/usr/bin/env node
// opencode_process_boundary.mjs — deterministic process-boundary harness for
// generated runtime adapters (OpenCode plugin, Pi/OMP extensions).
//
// Loads a generated adapter through its supported ESM module form after two
// test-only substitutions:
//   1. `node:child_process`'s spawnSync is replaced by a recording test
//      double (child_process_double.mjs) so the test observes the exact
//      command, input JSON, and options cwd the adapter uses at the process
//      boundary — no real child process, no source-substring assertions.
//   2. TS-only syntax in generated Pi/OMP extensions (`import type`, inline
//      type annotations) is erased so plain Node ESM can execute the module
//      body verbatim. The generated OpenCode plugin is already plain ESM.
//
// The rewritten module is written ADJACENT to the source plugin so
// `import.meta.url`-derived launcher paths resolve exactly as they would for
// a plugin relocated into a temporary installation (spec: Module-derived
// adapter assets).
//
// Usage:
//   node opencode_process_boundary.mjs \
//     --kind opencode|pi|omp \
//     --plugin <path to generated adapter> --cwd <process cwd> \
//     [--directory <string>] [--directory-json <raw json>] \
//     [--tool <name>] [--input-json <raw json>] [--args-json <raw json>] \
//     [--status <n>] [--stderr <text>] [--error <text>] [--throw <text>]
//
// Prints a single JSON object on stdout:
//   { "records": [ { "command", "input", "optionsCwd" } ],
//     "outcome": { "threw", "error", "block", "returned" } }
// and exits 1 with a JSON error object when the module cannot be loaded or
// driven (a loader/setup failure — the tests must never mistake it for a
// behavioral RED).

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join, basename, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const DOUBLE_URL = new URL("./child_process_double.mjs", import.meta.url).href;

function fail(message) {
  process.stdout.write(JSON.stringify({ records: [], outcome: { threw: false, error: message, block: false, returned: null } }));
  process.exit(1);
}

const args = process.argv.slice(2);
function opt(name) {
  const i = args.indexOf(name);
  return i >= 0 ? args[i + 1] : undefined;
}
function hasOpt(name) {
  return args.includes(name);
}

const kind = opt("--kind") ?? "opencode";
const pluginArg = opt("--plugin");
if (!pluginArg) fail("missing --plugin path");
// Resolve the plugin to an absolute path BEFORE chdir so a relative --plugin
// is anchored to the invoking process cwd, not the harness --cwd.
const pluginPath = resolve(pluginArg);
const cwd = opt("--cwd");
if (cwd) {
  try {
    process.chdir(cwd);
  } catch (err) {
    fail(`cannot chdir to --cwd ${cwd}: ${String(err?.message ?? err)}`);
  }
}
const directoryJson = hasOpt("--directory-json")
  ? JSON.parse(opt("--directory-json"))
  : undefined;
const directory = hasOpt("--directory")
  ? opt("--directory")
  : hasOpt("--directory-json")
    ? directoryJson
    : undefined;
const toolName = opt("--tool") ?? "Bash";
const inputJson = hasOpt("--input-json") ? JSON.parse(opt("--input-json")) : {};
const argsJson = hasOpt("--args-json") ? JSON.parse(opt("--args-json")) : {};
const double = {
  status: hasOpt("--status") ? Number(opt("--status")) : 0,
  stderr: opt("--stderr") ?? "",
  error: opt("--error"),
  throw: opt("--throw"),
};
globalThis.__WT_TEST_DOUBLE__ = double;
globalThis.__WT_RECORDS__ = [];

// --- load the generated module -------------------------------------------------

let source;
try {
  source = readFileSync(pluginPath, "utf8");
} catch (err) {
  fail(`cannot read plugin ${pluginPath}: ${String(err?.message ?? err)}`);
}

// Substitute the spawnSync double, then erase TS-only syntax so plain Node
// ESM can execute the module body.
const rewritten = source
  .replace('"node:child_process"', `"${DOUBLE_URL}"`)
  .replace(/^import\s+type\s+[^;]+;\s*$/gm, "")
  .replace(/: Record<string, string>/g, "")
  .replace(/: ExtensionAPI/g, "")
  .replace(/: any/g, "");

// The rewritten module must sit ADJACENT to the source plugin so that
// import.meta.url-derived launcher paths (../../ relative to the adapter's
// runtime subdirectory) resolve exactly as they would for a relocated plugin.
const dir = dirname(pluginPath);
const base = basename(pluginPath).replace(/\.(ts|mjs|js)$/i, "");
const outPath = join(dir, `wt-boundary-${base}.mjs`);
try {
  writeFileSync(outPath, rewritten);
} catch (err) {
  fail(`cannot write rewritten module: ${String(err?.message ?? err)}`);
}

let mod;
try {
  mod = await import(pathToFileURL(outPath).href);
} catch (err) {
  fail(`module load failed: ${String(err?.message ?? err)}`);
}

// --- drive the adapter ----------------------------------------------------------

const outcome = { threw: false, error: null, block: false, returned: null };

try {
  if (kind === "opencode") {
    const api = await mod.plugin({ project: { cwd: process.cwd() }, directory });
    const handler = api?.["tool.execute.before"];
    if (typeof handler !== "function") {
      fail('opencode plugin did not expose "tool.execute.before" handler');
    }
    await handler({ tool: toolName }, { args: argsJson });
  } else if (kind === "pi" || kind === "omp") {
    let registered = null;
    const fakePi = {
      on(event, cb) {
        if (event === "tool_call") registered = cb;
      },
    };
    mod.default(fakePi);
    if (typeof registered !== "function") {
      fail(`${kind} extension did not register a tool_call handler`);
    }
    const returned = registered({ toolName, input: inputJson });
    outcome.returned = returned ?? null;
    outcome.block = Boolean(returned && returned.block);
  } else {
    fail(`unknown --kind ${kind}`);
  }
} catch (err) {
  outcome.threw = true;
  outcome.error = String(err?.message ?? err);
}

process.stdout.write(JSON.stringify({ records: globalThis.__WT_RECORDS__, outcome }));
