# HTML PDF Reference

Use this reference when the user asks for a PDF with rich visual layout, tables, colors, print styling, page breaks, or business-report formatting.

## Tool Parameters

- `html`: Self-contained HTML body or full HTML document.
- `css`: Optional inline CSS appended to the document.
- `filename`: Optional display filename without path separators. The `.pdf` extension is added automatically.
- `title`: Optional title used when wrapping an HTML fragment. Visible title text must still be included in `html`.
- `lifecycle`: Optional `persistent` or `temporary`. Defaults to `persistent`.

## HTML Rules

- Use semantic HTML directly: headings, paragraphs, sections, tables, lists, and small inline elements.
- Use CSS for layout and print styling.
- Prefer `@page` for page size and margins.
- Use `.page-break { break-before: page; }` or inline `style="break-before: page"` for page breaks.
- Use `thead { display: table-header-group; }` when table headers should repeat across pages.
- Keep the document self-contained.

Example:

```html
<main class="report">
  <h1>Monthly Utility Statement</h1>
  <p>The electricity charge for this period is <strong class="amount">$113.47</strong>.</p>
  <table>
    <thead>
      <tr>
        <th>Item</th>
        <th>Quantity</th>
        <th>Unit Price</th>
        <th>Amount</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td>Electricity</td>
        <td>123.33</td>
        <td>0.92</td>
        <td>113.47</td>
      </tr>
    </tbody>
  </table>
</main>
```

Example CSS:

```css
@page {
  size: A4;
  margin: 18mm;
}
body {
  font-family: Arial, "DejaVu Sans", sans-serif;
  color: #111827;
}
h1 {
  text-align: center;
  font-size: 22px;
  margin: 0 0 18px;
}
.amount {
  color: #c00000;
}
table {
  width: 100%;
  border-collapse: collapse;
}
th, td {
  border: 1px solid #d1d5db;
  padding: 8px 10px;
}
thead {
  display: table-header-group;
}
tr {
  break-inside: avoid;
}
```

## Constraints

- This creates a new PDF file from HTML. It does not edit an existing PDF or preserve a user-uploaded PDF template.
- Do not pass JSON document specs. Use HTML directly.
- Do not reference external URLs, remote fonts, external stylesheets, scripts, iframes, forms, or remote assets.
- JavaScript is disabled during rendering.
- Use data URLs for small embedded images only when necessary.
- Do not promise interactive PDF features, comments, signatures, watermarks, forms, or exact browser-to-PDF fidelity beyond normal Chromium print rendering.
- Use `generate_file` with `format=pdf` instead when the user only needs plain text lines.
