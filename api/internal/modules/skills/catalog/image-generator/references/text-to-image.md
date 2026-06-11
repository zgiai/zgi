# Text To Image

## Use When

Use for plain text-to-image generation from a user prompt without an existing reference image.

## Payload Rules

- `prompt` must describe the subject, scene, composition, intended use, and visual details.
- `aspect_ratio` must be one of `1:1`, `16:9`, `9:16`, or `4:3`.
- `count` must be between 1 and 4.
- Use `negative_prompt` for prohibited elements, unwanted colors, text, logos, or visual defects.
- Use `style` to set broad visual direction, not to imitate a living artist.

## Prompt Guidance

Good prompts include:

- Main subject.
- Background or environment.
- Composition and camera angle.
- Lighting, palette, and mood.
- Intended use such as article header, poster, product concept, or icon.
- Elements to avoid.

## Clarification Rules

Ask before generating when the prompt is too vague, the aspect ratio is missing for a design asset, or the user requests commercial material involving brands or real people.
