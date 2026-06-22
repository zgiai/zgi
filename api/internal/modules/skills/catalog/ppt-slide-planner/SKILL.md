---
name: ppt-slide-planner
description: Plan precise slide-by-slide PPT structures from a topic, text content, requirements, or uploaded-material summaries, and prepare a file-generator-ready PPTX payload for downstream generation. Use when the user asks for PPT planning, PowerPoint outline, slide planning, page-by-page PPT requirements, presentation storyboard, report deck structure, pitch deck outline, training deck plan, proposal slides, project report slides, business review slides, or when the user asks to first create a precise outline before generating a PPT file. This skill is the planning and layout layer, not the PPTX file generator.
when_to_use: Use this skill when the user wants a PowerPoint/PPTX deck planned from a topic, notes, copied text, summarized source material, or agent-prepared outline; use it for presentation planning, slide-by-slide content design, page splitting, layout selection, brand styling, media placeholder planning, and preparing structured requirements for the file-generator skill.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 60
display:
  icon: presentation
  category: productivity
  label:
    en_US: PPT Slide Planner
    zh_Hans: PPT 规划器
  description:
    en_US: Plans precise slide-by-slide PPT content, layout, style, and placeholders before PPTX generation.
    zh_Hans: 先规划每页 PPT 的内容、版式、样式和占位，再交给文件生成器输出。
  when_to_use:
    en_US: Use when PPT content must be planned page by page before file generation.
    zh_Hans: 当需要先逐页规划 PPT 内容和排版，再生成文件时使用。
  tags:
    en_US:
      - PPT
      - Presentation
      - Slides
    zh_Hans:
      - PPT
      - PPT规划
      - 幻灯片编排
---

# PPT Slide Planner Skill

Use this skill to turn a user's topic, text content, requirements, or source-material summary into a strict per-slide construction plan. This skill plans the PPT only; the actual editable PPTX file is produced by the separate `file-generator` skill.

This skill must not call PPTX generation tools, render PPTX files by itself, implement fallback renderers, or bypass file-generator. When the user asks for a PPT file, first produce the planning output and then route that output to the `file-generator` skill, which owns `generate_pptx`.

## Workflow

1. Determine whether the user provided enough context: topic, audience, use case, slide count or duration, style or brand requirements, and source content.
2. If the topic, source content, audience, or use case is too unclear to produce a reliable deck, call `request_user_input` before planning. For slide count, style, and layout preferences, make conservative defaults when the content is otherwise clear and state the assumptions.
3. Read `deck-brief.md` and produce a concise internal deck brief.
4. Read `slide-plan-contract.md` and create a slide-by-slide plan. Every slide must have exact title text, body text, page objective, layout, element list, and media placeholder requirements.
5. Read `slide-layout-contract.md` and select a concrete layout for each slide.
6. Read `media-placeholder-contract.md` when the deck needs image, chart, diagram, or video placeholder areas.
7. Read `pptx-payload-contract.md` and convert the slide plan into one complete strict JSON object that can be passed later as `file-generator/generate_pptx.presentation`.
8. Read `quality-check.md` and self-check every slide before handoff. If any text element may overflow, increase `h`, reduce text, add line breaks, or split the content before preparing the payload.
9. Before handoff, verify `presentation` starts with `{`, ends with `}`, contains only strict JSON, has balanced braces/brackets, closes the `slides` array, and contains no Markdown fences, comments, prose, trailing commas, or unquoted visible text.
10. Return or hand off the strict slide plan and PPTX payload to the `file-generator` skill when file output is required. Do not call `generate_pptx` from this skill.
11. In the final answer, briefly mention that the PPT plan is ready for file generation and list any important assumptions. Do not paste the full presentation JSON unless the user explicitly asks for it.

## Clarification Workflow

When a blocking decision is missing or ambiguous, call `request_user_input` instead of writing a plain clarification message. Do not stop for every optional preference.

Ask about:

- Presentation topic when the topic is not clear.
- Audience or use case when the intended communication goal is unclear.
- Slide count or presentation duration only when the user explicitly requires a controlled length and the target size is unclear. Otherwise choose a compact slide count from the content volume and state the assumption.
- Style, template, or brand only when the user requires a specific template or corporate brand but does not provide enough brand rules. Otherwise use a neutral business style and state the assumption.
- Whether image, chart, diagram, or video placeholders are needed when the content implies visual materials.

After calling `request_user_input`, stop the turn and wait for the user's answer. Do not start PPTX file generation in the same turn.

## References

Read references only when needed, but always read `pptx-payload-contract.md` and `quality-check.md` before preparing a file-generator handoff payload.

| Task | Read reference |
| --- | --- |
| Build the deck brief from topic, audience, source content, and requirements | `deck-brief.md` |
| Create strict per-slide plans with exact text and element requirements | `slide-plan-contract.md` |
| Choose concrete slide layouts and coordinates | `slide-layout-contract.md` |
| Reserve image, chart, diagram, or video placeholder areas | `media-placeholder-contract.md` |
| Convert the slide plan into file-generator `generate_pptx.presentation` JSON | `pptx-payload-contract.md` |
| Validate text density, overlap, bounds, style consistency, and output readiness | `quality-check.md` |

## Output Contract

Before handing off to `file-generator`, internally prepare:

- `deck_brief`: theme, audience, scenario, slide count, style, language, brand rules, and source constraints.
- `slide_plan`: one object per slide with exact title, objective, layout, elements, text, coordinates, style, and placeholder descriptions.
- `pptx_payload`: valid strict `file-generator/generate_pptx.presentation` JSON string.

The downstream `file-generator/generate_pptx` call must pass:

- `presentation`: strict JSON string with `layout`, `language`, `default_style`, and `slides`.
- `filename`: concise ASCII filename without extension.
- `title`: optional title hint.
- `lifecycle`: `persistent` unless the user asks for temporary output.

## Constraints

- Do not call `generate_pptx` from this skill. Each slide must have a strict slide plan first, then the payload is handed to `file-generator`.
- Do not generate PPTX files directly from this skill. Always hand the final strict presentation JSON to the `file-generator` skill.
- Do not implement or assume a fallback PPTX renderer in the planning skill. If file generation infrastructure is unavailable, report that file generation must be retried through `file-generator` after the dependency is available.
- Do not invent facts, metrics, product claims, customer names, or conclusions not provided or clearly implied.
- Do not use unsupported PPTX features: animations, transitions, speaker notes, comments, slide masters, theme inheritance, real charts, embedded media, external images, or editing existing PPTX files.
- If image, chart, diagram, or video content is requested, create a visible placeholder using `shape` and `text`; do not claim the media is embedded.
- Do not pass Markdown, HTML, comments, prose, partial objects, or unquoted text as `presentation`; pass strict valid PPTX JSON only.
- Do not put raw multi-line text inside JSON string values. Encode line breaks as `\n`.
- Do not use trailing commas or single quotes in JSON.
- Do not prepare a truncated or partially closed JSON object. Balance `{}`, `[]`, close `slides`, and keep the final non-space character as `}`.
- Keep generated PPTX JSON compact. Prefer under 10,000 characters, no more than 8 slides, and no more than 10 elements per slide unless the user explicitly requires more.
- If downstream file generation reports `unexpected EOF`, rebuild a shorter complete JSON payload from the slide plan before handing off again.
- Split dense content into more slides. Do not shrink text until it becomes unreadable.
- Do not prepare a handoff payload with a body text element over 10 estimated wrapped lines or over `0.75 * h`.
- Do not prepare a handoff payload with a normal body text element above 4.0 inches estimated height unless the user explicitly requested a dense appendix slide.
- Never prepare a single body `text` element above 4.5 inches estimated height; split it into continuation slides.
- For near-full-height body boxes such as `h=5.0` or `h=5.3`, do not increase height after a fit failure. Split the text.
- For local text boxes in two-column, comparison-card, visual-placeholder, process-step, sidebar, or any body box with `w < 7.0` or `h < 4.2`, keep estimated height at or below `min(2.8, h * 0.70)`.
- For small cards with `h <= 3.5`, keep estimated height at or below `h * 0.65`; do not fix overflow by only increasing the local box height.
- For narrow columns with `w <= 5.8`, use at most 3 bullets or about 90 Chinese characters / 220 English characters per text element.
- If downstream file generation reports `text does not fit`, split the reported slide/element into continuation slides before handing off a new payload.
- For `title_body` body boxes at `h=5.3`, overflow must be fixed by splitting the slide, not by increasing height.
- Do not use small title boxes for long titles. A 36-44 pt title that may wrap to two lines needs at least `h=1.6`; a one-line title needs at least `h=0.85`.
- For every title/text element, reserve height from text length and font size before handoff; set `h` to at least `estimated_height * 1.25`.
- Do not prepare a handoff payload when estimated text height is within `0.2` inches of `h`; increase `h` by at least `0.3` inches or shorten/split text first.
- Do not use `h < 1.15` for 16-24 pt subtitle, label, or metadata text unless it is one short line with estimate under `0.65`.
- Never rely on the renderer to shrink overflowing text.
- Keep readable text and tables within slide bounds and non-overlapping.
- Use consistent font, colors, title hierarchy, and spacing across the deck.
- If the user's requirements exceed current PPTX support, state the limitation and prepare the closest supported editable static deck only when it still satisfies the request.
