# Portable script Skills in ZGI

ZGI accepts a ZIP containing an [Agent Skills](https://agentskills.io/specification) `SKILL.md` and optional `scripts/`, `references/`, and `assets/` directories. A ZGI `skill.manifest.json` is not required for the console Agent's session-script mode. If `SKILL.md` omits `name`, ZGI uses the ZIP's single top-level directory name (or the ZIP filename when files are at the root).

```text
csv-summary/
  SKILL.md
  scripts/summarize.py
  references/usage.md
```

The ZIP must satisfy the existing 64 MiB import limit. Files must be regular files with safe relative paths; symlinks and path traversal are rejected. Import preview reports the execution mode, dependency hints, and unsupported extensions. Existing ZGI manifest packages and manifest-free `scripts/run.py` packages retain their `run_script` behavior.

## Run in an approved console Agent

Enable the Skill on an authenticated console Agent with an approved `execution_runtime` profile. The runtime must provide offline Linux Shell and file transfer. Other Agent entry points and API keys do not gain Shell access.

Call `load_skill` to read the Skill's instructions. For a portable script Skill, its result includes `session_script.workspace_path`, `package_sha256`, and the required execution tool. The path is relative to the conversation workspace, not the API host. When calling `shell_exec`, pass the same `skill_id`: ZGI then places the package in the session before running the command and sets `ZGI_SKILL_DIR` to its absolute directory. The package is staged once per version and re-staged after a workspace rebuild.

For example, a Skill can instruct the Agent to run:

```json
{"skill_id":"csv-summary","command":"python3 \"$ZGI_SKILL_DIR/scripts/summarize.py\" inputs/data.csv outputs/summary.json"}
```

For Node.js, use `node "$ZGI_SKILL_DIR/scripts/summarize.js" ...`. Uploaded user files are under `inputs/` for the current turn. A Skill may read its own `references/` and `assets/` through `$ZGI_SKILL_DIR`. Write generated files outside the versioned Skill directory, for example under `outputs/`, then call `workspace_export_file` with the output path to deliver a download card. `shell_exec` returns exit code, stdout, and stderr; non-zero exits and missing modules are visible diagnostics. No third-party packages are installed automatically.

Do not call ZGI's `run_script` tool for a portable Skill: that tool uses a separate manifest/input protocol. Conversely, legacy packages can continue to use it unchanged. Offline execution does not provide network access, OS package installation, Bash-specific features, Claude Code dynamic context injection, or other host-specific extensions. Declare required dependencies in the Skill instructions and use a prebuilt approved runtime profile when they are needed.
