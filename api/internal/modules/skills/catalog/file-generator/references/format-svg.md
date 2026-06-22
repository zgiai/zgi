# SVG Output

Use `generate_file` with `format: "svg"` when the user asks for an SVG or editable vector image file.

## Content Rules

- Provide a complete SVG document, starting with `<svg ...>` and ending with `</svg>`.
- Include `xmlns="http://www.w3.org/2000/svg"` on the root element.
- Set a clear `viewBox`; add `width` and `height` only when the user asks for fixed dimensions.
- Prefer simple vector shapes, paths, text, gradients, and groups.
- Keep the SVG self-contained. Do not reference external images, fonts, scripts, CSS files, or network URLs.
- Do not wrap SVG content in HTML.

## Example

```xml
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 480">
  <rect width="800" height="480" fill="#f8fafc"/>
  <circle cx="400" cy="220" r="96" fill="#2563eb"/>
  <text x="400" y="390" text-anchor="middle" font-size="32" fill="#0f172a">ZGI</text>
</svg>
```
