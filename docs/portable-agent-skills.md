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

Enable the Skill on an authenticated console Agent with an approved `execution_runtime` profile. The default runtime provides offline Linux Shell and file transfer. A separately approved local gVisor `controlled-http` profile may also provide a loopback HTTPS proxy restricted to its operator-defined host allowlist. Other Agent entry points and API keys do not gain Shell access.

In the console Agent's native loop, ZGI provides an active Skill's instructions and `session_scripts` metadata directly; a not-yet-active Skill is made available through `activate_skills`. Do not call `load_skill` in that loop. The metadata includes the Skill ID, a conversation-workspace path, and `package_sha256`. When calling `shell_exec`, pass the matching `skill_id` and exact 64-character digest from `session_scripts`: ZGI verifies the package version before placing it in the session and sets `ZGI_SKILL_DIR` to its absolute directory. The package is staged once per version and re-staged after a workspace rebuild. If the Skill is replaced between activation and execution, the call fails with `skill_version_changed`; start a new turn to obtain the updated Skill version. The legacy Skill loop still exposes `load_skill` and its `session_script` result.

For example, a Skill can instruct the Agent to run:

```json
{"skill_id":"csv-summary","package_sha256":"<package_sha256 from session_scripts>","command":"python3 \"$ZGI_SKILL_DIR/scripts/summarize.py\" inputs/data.csv outputs/summary.json"}
```

For Node.js, use `node "$ZGI_SKILL_DIR/scripts/summarize.js" ...`. Uploaded user files are under `inputs/` for the current turn. A Skill may read its own `references/` and `assets/` through `$ZGI_SKILL_DIR`. Write generated files outside the versioned Skill directory, for example under `outputs/`, then call `workspace_export_file` with the output path to deliver a download card. `shell_exec` returns exit code, stdout, and stderr; non-zero exits and missing modules are visible diagnostics. No third-party packages are installed automatically.

`ZGI_SKILL_DIR` is the complete package root. Use the paths documented by the Skill, not an assumed `scripts/` prefix. If a helper requires Python local module imports, run from the package directory, for example `cd "$ZGI_SKILL_DIR" && python -c 'from core.gif_builder import GIFBuilder'`. Use absolute `/workspace/inputs/...` and `/workspace/outputs/...` paths after changing directories.

Do not call ZGI's `run_script` tool for a portable Skill: that tool uses a separate manifest/input protocol. Conversely, legacy packages can continue to use it unchanged. Offline execution does not provide network access or dependency installation. Bash-specific features, Claude Code dynamic context injection, and other host-specific extensions are not a general compatibility promise.

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
