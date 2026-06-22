# Deck Brief Reference

Use this reference before creating the slide plan.

## Required Brief Fields

Create an internal `deck_brief` with:

- `topic`: the presentation subject.
- `audience`: who will read or watch the deck.
- `scenario`: business report, project proposal, training, pitch, work summary, review, or other user-provided context.
- `goal`: what the deck should make the audience understand or decide.
- `source_summary`: concise summary of the supplied text, notes, or uploaded-material summary.
- `slide_count`: target number of slides. If unknown, choose a compact count based on content volume and state the assumption unless the user explicitly requires a controlled length.
- `language`: default to the user's language.
- `style`: business, technology, minimal, corporate brand, or user-provided style.
- `brand`: font, colors, logo rules, and template requirements when provided.
- `visual_needs`: image, chart, diagram, table, video, or other placeholders required by the content.

## Rules

- Preserve user-provided facts and terminology.
- Do not invent data, metrics, customer names, product names, or conclusions.
- If the source is long, group it by themes before creating slides.
- Ask for clarification only when topic, source content, audience, or use case is too unclear to produce a reliable deck.
- If slide count, duration, or style is unspecified but the content is otherwise clear, use conservative defaults and state the assumptions.
- If the user requires a specific corporate brand but does not provide brand colors or font, ask for them. If no specific brand is required, use a neutral business default.
- If uploaded files are mentioned but their content is not available in context, ask for extracted text or a summary before generating the deck.

## Brief Example

```json
{
  "topic": "AI customer service platform upgrade plan",
  "audience": "company leadership",
  "scenario": "project proposal",
  "goal": "explain why the upgrade is needed and obtain approval for phased delivery",
  "slide_count": 8,
  "language": "zh-CN",
  "style": "business technology",
  "brand": {
    "font_face": "Microsoft YaHei",
    "primary_color": "1F4E79",
    "accent_color": "2F80ED"
  },
  "visual_needs": ["process placeholder", "comparison table"]
}
```
