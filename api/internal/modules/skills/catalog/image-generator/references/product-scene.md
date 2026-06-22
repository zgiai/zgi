# Product Scene

## Use When

Use for product concept images, ecommerce scene images, lifestyle product visuals, packaging concepts, and product-in-use renderings.

## Payload Rules

- Use `generate_image` for concept images.
- Use `edit_image` when a product reference image is provided.
- Include product type, material, target user, use environment, camera angle, and lighting.
- Use `negative_prompt` to avoid wrong logos, extra labels, distorted packaging, or unreadable text.

## Data Rules

- Do not claim the image is an accurate product photo. The current tool uses prompt-plus-reference-URL regeneration and does not expose faithful structured product-image editing.
- Remind the user to confirm product, trademark, and advertising compliance before commercial use.

## Clarification Rules

Ask when the product appearance, target scene, or commercial use context is unclear.
