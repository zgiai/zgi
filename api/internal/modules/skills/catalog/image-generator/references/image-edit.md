# Image Edit

## Use When

Use for background changes, color changes, adding elements, removing elements, style transfer, and local-edit-like requests based on a reference image. The backend currently uses prompt-plus-reference-URL regeneration, not structured image editing.

## Payload Rules

- Use `edit_image`.
- `image` and `edit_instruction` are required.
- Choose `edit_type`: `background`, `color`, `add_element`, `remove_element`, `style_transfer`, `variant`, or `auto`.

## Edit Instruction Rules

Write edit instructions with:

- Target area or element.
- Desired change.
- Elements to preserve.
- Elements to avoid.
- Style or lighting constraints.

## Clarification Rules

Ask when the edit target is unclear, the user says "modify this" without details, or the edit involves a real person, brand logo, product packaging, or rights-sensitive content.
