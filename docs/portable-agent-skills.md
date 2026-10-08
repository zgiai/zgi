# Portable script Skills in ZGI

ZGI accepts a ZIP containing an [Agent Skills](https://agentskills.io/specification) `SKILL.md` and optional `scripts/`, `references/`, and `assets/` directories. A ZGI `skill.manifest.json` is not required for the console Agent's session-script mode. If `SKILL.md` omits `name`, ZGI uses the ZIP's single top-level directory name (or the ZIP filename when files are at the root).

Python and Node helpers do not have to live in `scripts/`. Regular `.py`, `.js`, `.mjs`, and `.cjs` files at the package root or under directories such as `core/` and `lib/` also enable session-script discovery. This is not an execution permission: the approved console runtime, package integrity checks, and safe archive restrictions still apply. Built-in Skill discovery and legacy `scripts/run.py` classification are unchanged.

```text
csv-summary/
  SKILL.md
  scripts/summarize.py
  references/usage.md
```

The ZIP must satisfy the existing 64 MiB import limit. Files must be regular files with safe relative paths; symlinks and path traversal are rejected. Import preview reports the execution mode, dependency hints, and unsupported extensions. Existing ZGI manifest packages and manifest-free `scripts/run.py` packages retain their `run_script` behavior.

## Run in an approved console Agent

Enable the Skill on an authenticated console Agent and turn on its Linux execution environment. Users do not select Profile, Policy, Snapshot or network settings. The operator-approved standard runtime provides Shell, file transfer and explicit package installation through controlled package sources; legacy offline runtimes have no network or installation capability. A separately approved legacy `controlled-http` runtime may provide allowlisted HTTPS access. Other Agent entry points and API keys do not gain Shell access. See [setup and use](agent-runtime-quickstart.md) for deployment examples.

In the console Agent's native loop, ZGI provides an active Skill's instructions and `session_scripts` metadata directly; a not-yet-active Skill is made available through `activate_skills`. Do not call `load_skill` in that loop. The metadata includes the Skill ID, a conversation-workspace path, and `package_sha256`. When calling `shell_exec`, pass the matching `skill_id` and exact 64-character digest from `session_scripts`: ZGI verifies the package version before placing it in the session and sets `ZGI_SKILL_DIR` to its absolute directory. The package is staged once per version and re-staged after a workspace rebuild. If the Skill is replaced between activation and execution, the call fails with `skill_version_changed`; start a new turn to obtain the updated Skill version. The legacy Skill loop still exposes `load_skill` and its `session_script` result.

For example, a Skill can instruct the Agent to run:

```json
{"skill_id":"csv-summary","package_sha256":"<package_sha256 from session_scripts>","command":"python3 \"$ZGI_SKILL_DIR/scripts/summarize.py\" inputs/data.csv outputs/summary.json"}
```

For Node.js, use `node "$ZGI_SKILL_DIR/scripts/summarize.js" ...`. Uploaded user files are under `inputs/` for the current turn. A Skill may read its own `references/` and `assets/` through `$ZGI_SKILL_DIR`. Write generated files outside the versioned Skill directory, for example under `outputs/`, then call `workspace_export_file` with the output path to deliver a download card. `shell_exec` returns exit code, stdout, and stderr; non-zero exits and missing modules are visible diagnostics. No third-party packages are installed automatically.

Commands run under `/bin/sh -c`. Preserve the injected `PATH` and `HOME`; a nested login shell such as `bash -lc` can reset them and hide retained session dependencies. Use POSIX syntax by default, or `bash -c` explicitly for Bash-only syntax. For required sequential steps use `set -eu` or `&&`, and preserve the original exit status instead of appending `echo`, `exit 0`, or `|| true`. A successful tool call is not evidence of a successful script: check the terminal Job status and `exit_code`, and inspect stderr. The console displays Shell Job results separately from tool transport success; a missing terminal Job status or exit code is not shown as confirmed success. Independent Shell calls retain separate timeline records, even if a later call succeeds or their safe argument summaries match.

This display reflects the Shell's actual exit status, not the correctness of each inner command. A command that intentionally or accidentally suppresses an error and returns zero still appears successful; inspect diagnostics and validate the resulting files. Prompt guidance reduces this risk but cannot guarantee that every generated command preserves failures. In the current Worker contract, a timeout is a failed Job with `exit_code: -1` and `error_code: job_failed`; cancellation is `canceled`. Neither is successful script execution.

`ZGI_SKILL_DIR` is the complete package root. Use the paths documented by the Skill, not an assumed `scripts/` prefix. If a helper requires Python local module imports, run from the package directory, for example `cd "$ZGI_SKILL_DIR" && python -c 'from core.gif_builder import GIFBuilder'`. Use absolute `/workspace/inputs/...` and `/workspace/outputs/...` paths after changing directories.

Do not call ZGI's `run_script` tool for a portable Skill: that tool uses a separate manifest/input protocol. Conversely, legacy packages can continue to use it unchanged. Installation depends on the current Binding's reported capabilities: offline Bindings cannot install, while approved standard package-install Bindings can. POSIX shell is the default; invoke `bash -c` explicitly when needed and available. Claude Code dynamic context injection and other host-specific extensions are not a general compatibility promise.

When the operator enables the standard runtime's `package-install` capability, the model can explicitly install a diagnosed missing Python, Node, or approved system package through the supplied `pip`, `npm`, or `apt-get` commands; no ZGI dependency declaration is required. Package sources remain controlled, Jobs remain non-root, and the approved installation helper handles system packages inside the sandbox. Arbitrary external network access and unrestricted root operations are not enabled. Installed dependencies use the retained artifact/ledger recovery path; failures must be reported rather than silently downloading latest versions during recovery. See [dependency installation and retention](agent-runtime-m6b-dependency-preparation.md) for deployment controls and limits. Legacy controlled-HTTPS profiles may instead expose `workspace_prepare_dependencies` for their declarative Python/Node preparation path.

## Regression checks

To test an unmodified external package without committing third-party files, run from `api/` with the repository's Go version:

```bash
ZGI_PORTABLE_SKILL_TEST_ROOT=/absolute/path/to/unpacked-skill \
  go test ./internal/capabilities/chatruntime/service \
  -run '^TestImportExternalPortableSkill$' -count=1 -v
```

This checks import preview and confirmation only. It does not prove backend execution or network isolation. Run the following console acceptance task separately on each approved backend, using a dedicated test account and unchanged package content:

| Skill category | Fixed input and expected result |
| --- | --- |
| Spreadsheet | Write values 10 and 20 with `SUM(B2:B3)`; run the bundled recalculation helper; verify the formula remains and the cached value is 30 with no formula errors. |
| Presentation | Generate two slides containing a unique test marker and a native chart with values 10 and 20; run bundled validation, PDF conversion, and thumbnail helpers; independently inspect chart XML and rendered pages. |
| Animation | Import the package's original local GIF modules, including packages with only `core/`; create a 128 × 128 GIF with 12 distinct frames at 100 ms per frame; run bundled validation. |

For all three, verify non-root execution, retain real errors, install only diagnosed missing dependencies through approved commands, and export results with `workspace_export_file`. Download the files through the console and independently check their contents and SHA256 values. Record package hashes to establish that the original modules were used.

Then checkpoint the idle environment, simulate loss of only the dedicated test compute, and resume the same conversation. Without regenerating files or running installation commands, verify the loaded packages, installed module versions, and every output checksum. Keep checkpoints and test evidence until reviewed. Mock tests and successful import alone are not substitutes for this recovery test.

### Automated real-backend matrix

`TestAgentExecutionStandardSkillMatrix` runs unchanged `xlsx`, `pptx`, and `slack-gif-creator` package helpers from an external checkout. It requires a dedicated authenticated Worker with the standard runtime and approved `package-install` capability. It creates and deletes its own test Binding; do not point it at a production Worker or replace its package-only network policy with unrestricted network access. Set the required API application credentials through your normal local test environment, without putting credentials in command arguments or Git.

```bash
export ZGI_STANDARD_SKILL_TEST_ROOT=/absolute/path/to/skills
export ZGI_AGENT_RUNTIME_TEST_URL=http://dedicated-worker:2671
export ZGI_AGENT_RUNTIME_TEST_PROFILE=cc-codex-standard
export ZGI_AGENT_RUNTIME_TEST_BACKEND=local-gvisor
# Set ZGI_AGENT_RUNTIME_TEST_API_KEY securely in the environment.
# Optionally set ZGI_STANDARD_SKILL_TEST_OUTPUT to an ignored output directory.
go test -tags=agent_runtime_integration ./internal/capabilities/agentexecution/runtime \
  -run '^TestAgentExecutionStandardSkillMatrix$' -count=1 -v -timeout=15m
```

Run from `api/` using the repository's Go version. Repeat against the approved E2B Worker with its configured Profile ID and `ZGI_AGENT_RUNTIME_TEST_BACKEND=e2b`. The matrix verifies real Shell execution, original package hashes, spreadsheet formula/cache values, native chart data, PDF conversion, GIF frames, file export, nonzero exit visibility, missing-package diagnostics, registry override/global-install rejection, timeout/cancellation with descendant cleanup, cross-organization rejection, and checkpoint/resume retention. It does **not** replace browser import/download acceptance, network-bypass testing, or simulated compute-loss recovery. The UI-specific Job presentation check is `pnpm test:agent-execution-job-presentation` from `web/`.

Use the Sandbox's existing `TestGVisorPackageInstallRecovery` and `TestE2BPackageInstallContract` real-backend tests for native Python/Node builds, approved system packages, network bypass rejection, compute-loss recovery, and corrupt-artifact/ledger rejection. Recovery must fail closed when retained dependencies are damaged: repair the retained data through the operator recovery workflow, then retry; do not silently resolve different or latest package versions. Keep the existing legacy Script Skill E2E and Linux secure-backend gates separate from the session-runtime matrix.
