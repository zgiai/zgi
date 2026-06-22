# Reference Variant

## Use When

Use when the user provides a reference image and asks for variants, alternatives, similar concepts, style variations, or "based on this image". The backend currently provides the reference as a signed URL in the prompt, not as a structured image input.

## Payload Rules

- Use `edit_image`, not `generate_image`, when a reference image is central to the request.
- `image` is required.
- `edit_instruction` should state what to preserve and what may change.
- Use `edit_type: variant` unless the user requested a more specific edit.
- Include `style`, `aspect_ratio`, and `negative_prompt` when the user provides them.

## Data Rules

- Preserve only user-authorized source elements.
- Do not claim exact identity, brand, or character continuity unless the source and rights are clear.
- Explain that exact visual matching is not guaranteed because the reference is prompt URL guidance.

## Clarification Rules

Ask when no reference image is available, or when the user does not specify what should remain unchanged versus what should vary.
