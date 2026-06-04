# Rich DOCX Reference

Use this reference when the user asks for a Word document with styling, layout, headings, font control, paragraph alignment, page breaks, or simple tables.

## Tool Parameters

- `document`: JSON string describing the DOCX document.
- `filename`: Optional display filename without path separators. The `.docx` extension is added automatically.
- `title`: Optional title metadata. Visible title text must still be included in `document.blocks`.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## Document JSON

The JSON root object supports:

- `page`: optional page settings.
- `default_style`: optional default text style.
- `blocks`: required array of document blocks.

Supported block types:

- `heading`: title-like text with `level` 1, 2, or 3.
- `paragraph`: paragraph text or rich runs.
- `table`: simple table with optional headers and rows.
- `page_break`: page break.

Example:

```json
{
  "page": {
    "size": "a4",
    "orientation": "portrait",
    "margins": { "top": 72, "right": 72, "bottom": 72, "left": 72 }
  },
  "default_style": {
    "font_family": "Arial",
    "font_size": 12,
    "line_spacing": 1.5
  },
  "blocks": [
    {
      "type": "heading",
      "level": 1,
      "text": "Monthly Utility Statement",
      "style": {
        "font_family": "Arial",
        "font_size": 18,
        "bold": true,
        "alignment": "center"
      }
    },
    {
      "type": "paragraph",
      "runs": [
        { "text": "The electricity charge for this period is " },
        { "text": "$113.47", "bold": true, "color": "C00000" }
      ]
    },
    {
      "type": "table",
      "headers": ["Item", "Quantity", "Unit Price", "Amount"],
      "rows": [
        ["Electricity", "123.33", "0.92", "113.47"]
      ],
      "style": {
        "alignment": "center"
      },
      "table_style": {
        "border": true,
        "border_color": "000000",
        "width": 420
      }
    }
  ]
}
```

## Supported Style Fields

Text and run styles:

- `font_family`
- `east_asia_font`
- `ascii_font`
- `font_size`
- `bold`
- `italic`
- `underline`: boolean or underline style such as `single`, `double`, or `wave`
- `color`: `RRGGBB` or `#RRGGBB`
- `highlight`: Word highlight color such as `yellow`, `green`, or `cyan`

Paragraph styles:

- `alignment`: `left`, `center`, `right`, `justify`, or `distribute`
- `line_spacing`
- `space_before`
- `space_after`
- `indent_left`
- `first_line_indent`

Table cell objects may use:

- `text`
- `runs`
- `style`
- `background_color`
- `vertical_align`: `top`, `center`, or `bottom`

Table blocks may use:

- `header_bold`: whether header cells are bold. Defaults to true.
- `column_widths`: column widths in points.
- `table_style.alignment`: table alignment.
- `table_style.width`: table width in points.
- `table_style.border`: whether table borders are shown. Defaults to true.
- `table_style.border_color`: border color.

## Constraints

- This creates a new DOCX file. It does not edit an existing DOCX or preserve a user-uploaded template.
- Do not use Markdown or HTML as `document`; pass JSON.
- Every item in a `runs` array must include non-empty `text`. Omit empty runs instead of using `{"text":""}`.
- Use a paragraph with visible `text` for blank-line labels or separators; do not create heading or paragraph blocks that contain only empty runs.
- Tables are simple rectangular tables. Do not promise complex merged cells, formulas, charts, images, headers, footers, comments, revisions, or automatic table of contents.
- Use `generate_file` with `format=docx` instead when the user only needs plain text paragraphs.
