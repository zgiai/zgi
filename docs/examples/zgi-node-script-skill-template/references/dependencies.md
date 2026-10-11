# Dependencies

Read this reference when adding or moving code dependencies.

- `bundled_packages` documents code shipped inside the Skill archive. It does not
  install the package or add aliases to Node.js module resolution.
- Local Node.js packages must be imported with a relative specifier. This template
  uses `require("./zgi_skill_node_template")` from `scripts/run.js`.
- Third-party packages belong in a pinned root `package.json` or the manifest
  `dependencies.nodejs` list and must exist in the selected runtime Profile.
- System packages belong in `dependencies.system`; package declarations never grant
  permission to install software during a Skill invocation.
- Network access and runtime package installation remain unavailable by default.
