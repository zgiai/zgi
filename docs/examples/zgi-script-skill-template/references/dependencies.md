# Dependencies

Read this reference when adding or moving code dependencies.

- `bundled_packages` documents code shipped inside the Skill archive. It does not
  install the package or change Python's import path.
- Python packages must resolve from the directory containing the manifest entrypoint.
  This template declares `scripts/zgi_skill_template`, which `scripts/run.py` can
  import using normal Python semantics.
- Third-party Python packages belong in a pinned `requirements.txt` or the manifest
  `dependencies.python` list and must exist in the selected runtime Profile.
- System packages belong in `dependencies.system`; package declarations never grant
  permission to install software during a Skill invocation.
- Network access and runtime package installation remain unavailable by default.
