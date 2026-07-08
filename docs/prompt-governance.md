# Prompt Governance

This document defines the production behavior for ZGI Prompt assets.

## Asset Ownership

- Official prompts are read-only product templates. Users can test them, optimize them, or copy them into personal prompts before editing.
- Personal prompts belong to the creator and can be shared into the workspace.
- Workspace prompts are shared assets. Editing prompt content creates a new version; metadata changes do not change version content.

## Version Semantics

- Saving prompt content always creates a new version. Existing versions are not overwritten.
- The newest saved version is the latest version.
- The online version is the team-validated stable version marker. It must be set explicitly after validation.
- Content already copied into an Agent or workflow node does not change automatically when a prompt version changes.

## Workflow Usage

- New prompt library selection copies the selected version into the current LLM node as editable node content.
- Choosing another prompt template replaces the node content only after the user confirms the copy action.
- The prompt library remains unchanged when a workflow node is edited.

## Legacy Managed References

Older workflows may still contain `prompt_source: "managed"` and `prompt_reference`.

- Default bundled workflow prompts are normalized to inline copies automatically when the workflow loads.
- Non-default managed references remain visible as legacy managed references.
- Users should convert legacy managed references to node copies before publishing production workflows.
- Conversion keeps the current node prompt content, clears `prompt_reference`, and requires saving the workflow to persist.

## Release Governance

- Setting an online version requires prompt management permission.
- The online action changes the recommended stable version for future prompt selection.
- Existing copied node content is not affected by changing the online version.
- Only legacy managed references that follow the online label can use the new online version on their next run.

## Review Checklist

- Create a personal prompt with minimal fields.
- Copy an official prompt into a personal prompt.
- Edit prompt content and verify a new version is created.
- Set a validated version as the online version.
- Select a prompt from a workflow node and verify it copies into editable node content.
- Convert a legacy managed reference into a node copy.
- Save and reload the workflow to verify the copied prompt remains inline.
- Run prompt playground feedback on a realistic input.
