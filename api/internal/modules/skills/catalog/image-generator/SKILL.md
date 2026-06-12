---
name: image-generator
description: Generate image assets from text prompts or create reference-image variants and edit-style regenerated images for article illustrations, poster concepts, product concept art, marketing materials, ecommerce product scenes, illustrations, icons, and cover drafts. Use when the user asks for 生成图片, 图片生成, 文生图, 文章配图, 活动海报草图, 产品概念图, 营销素材, 插画生成, 图标草案, 封面草案, 电商商品场景图, 参考图生成, 图片变体, 换背景, 改颜色, 加元素, 去元素, generate image, text to image, image variant, image edit, background change, color change, add element, or remove element.
when_to_use: Use this skill when the answer should include generated image files from a prompt or reference-image edit request. Use generate_image for text-to-image and prompt-based candidates. Use edit_image for reference-image variants, background changes, color changes, adding elements, removing elements, and style transfer requests. Do not use this skill for OCR, image understanding, table extraction, screenshot diagnosis, or image-to-text analysis. For casual, vague, incomplete, or non-professional image generation requests, first route through prompt-professionalizer to optimize the prompt, then call this skill with the optimized prompt. For generic image generation requests, call request_user_input before generating.
provider_type: builtin
provider_id: image_generator
runtime_type: tool
tools:
  - generate_image
  - edit_image
max_calls_per_turn: 5
timeout_seconds: 120
display:
  icon: image-plus
  category: creative
  label:
    en_US: Image Generator
    zh_Hans: 图片生成
  description:
    en_US: Generates image assets and reference-image variants from prompts.
    zh_Hans: 根据描述生成图片素材，并支持参考图变体与编辑式重生成。
  when_to_use:
    en_US: Use when the answer should include generated image files.
    zh_Hans: 当回答需要生成图片文件时使用。
  tags:
    en_US:
      - Image
      - Generation
      - Creative
    zh_Hans:
      - 图片
      - 生成
      - 设计
---

# Image Generator Skill

Use this skill to generate image assets from user descriptions and to create reference-URL-guided variants or edit-style regenerated images. This skill creates image files; it does not analyze images, OCR text, extract tables, or diagnose screenshots.

## Supported Tasks

- `generate_image`: text-to-image generation for article illustrations, marketing assets, poster concepts, product concepts, ecommerce scenes, illustrations, icons, and cover drafts.
- `edit_image`: reference-URL-guided variants and edit-style regeneration such as background change, color change, adding elements, removing elements, and style transfer.

Important: the current backend does not pass reference images as structured image-edit inputs through `adapter.ImageRequest`; it passes a signed reference image URL inside the prompt. Treat `edit_image` as prompt-plus-reference-URL regeneration, not precise in-place editing or guaranteed reference adherence.

## Workflow

1. Determine whether the user wants text-to-image generation or reference-image editing/variants.
2. If the user request is casual, vague, incomplete, or not already written as a professional image prompt, first use `prompt-professionalizer` to produce an optimized image prompt and parameter suggestions.
3. If required image decisions are still missing after prompt professionalization, call `request_user_input` before calling any image tool.
4. Read exactly one reference document for the selected task or use case.
5. Build a concise generation prompt that includes subject, scene, composition, style, color, lighting, intended use, and prohibited elements when provided.
6. Choose or confirm aspect ratio and number of candidates.
7. Call `call_skill_tool` with `tool_name` set to `generate_image` or `edit_image`.
8. In the final answer, mention the generated file names, usage notes, and any copyright or brand compliance reminder. Do not paste raw image bytes or base64.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when a missing decision blocks useful generation. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- The user only says "生成图片", "做张图", "generate an image", or similar without a usable prompt.
- The intended use is unclear and affects composition, such as article image versus poster versus ecommerce scene.
- The aspect ratio is missing.
- The style is missing and the user's request implies a visual direction matters.
- The user asks for a reference-image variant but no reference image is available.
- The user asks for local editing but does not specify the edit target or desired result.
- The request involves brand logos, real people, product photos, commercial use, or likely rights-sensitive material.

Example `request_user_input` payload:

```json
{
  "message": "我可以生成图片，但需要先确认关键参数。",
  "questions": [
    {
      "id": "usage",
      "question": "图片主要用于什么场景？",
      "options": [
        { "label": "文章配图", "description": "偏叙事和视觉吸引。" },
        { "label": "活动海报", "description": "偏醒目构图和传播感。" },
        { "label": "产品概念图", "description": "突出产品形态和使用场景。" },
        { "label": "营销素材", "description": "突出卖点和品牌调性。" },
        { "label": "电商场景图", "description": "突出商品和环境。" }
      ]
    },
    {
      "id": "aspect_ratio",
      "question": "需要什么尺寸比例？",
      "options": [
        { "label": "1:1", "description": "适合头像、商品主图、社媒方图。" },
        { "label": "16:9", "description": "适合文章头图、横版展示。" },
        { "label": "9:16", "description": "适合手机海报、短视频封面。" },
        { "label": "4:3", "description": "适合通用展示图。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read exactly one reference after choosing the task:

| Requested task | Read reference |
| --- | --- |
| Text-to-image, generic image generation, illustration | `text-to-image.md` |
| Reference image variant, keep main subject, create alternatives | `reference-variant.md` |
| Background change, color change, add/remove element, style transfer | `image-edit.md` |
| Marketing asset, social media visual, campaign creative | `marketing-material.md` |
| Poster concept, cover draft, event key visual | `poster-concept.md` |
| Product concept, ecommerce product scene, product lifestyle image | `product-scene.md` |
| Style choice, realistic, illustration, flat, 3D, guofeng, tech | `style-guide.md` |

## Tool Usage

`generate_image` accepts:

- `prompt`: required image description.
- `style`: optional style: `auto`, `realistic`, `illustration`, `flat`, `3d`, `guofeng`, `tech`, `poster`, `product`, `icon`, or `cover`.
- `aspect_ratio`: optional ratio: `1:1`, `16:9`, `9:16`, or `4:3`. Defaults to `1:1`.
- `count`: optional number of candidates, 1-4. Defaults to 1.
- `negative_prompt`: optional prohibited elements or undesired visual traits.
- `reference_image`: optional current-user reference image file object or file ID. The tool places a signed reference image URL in the prompt for loose visual guidance; it is not a structured image input.
- `filename`: optional base filename without path separators.
- `lifecycle`: optional file lifecycle: `persistent` or `temporary`. Defaults to `persistent`.
- `provider` and `model`: optional explicit image model selection; usually omit.

`edit_image` accepts:

- `image`: required current-user reference image file object or file ID. The tool places a signed reference image URL in the prompt for loose visual guidance; it is not a structured image input.
- `edit_instruction`: required edit or variant instruction.
- `edit_type`: optional: `auto`, `variant`, `background`, `color`, `add_element`, `remove_element`, or `style_transfer`.
- `style`, `aspect_ratio`, `count`, `negative_prompt`, `filename`, `lifecycle`, `provider`, and `model`: same as `generate_image`.

## Constraints

- Before calling `generate_image` or `edit_image`, use `prompt-professionalizer` when the user's request is casual, vague, incomplete, or not already a professional image prompt. Direct tool calls are allowed only when the prompt and key parameters are already complete.
- Do not generate illegal, pornographic, exploitative, hateful, fraudulent, or clearly harmful imagery.
- Do not generate graphic violence or sexual content.
- Do not generate or imitate a specific living person's likeness.
- Do not create deceptive official documents, IDs, receipts, signatures, watermarks, or brand impersonation materials.
- For brand logos, recognizable people, product photos, or commercial assets, remind the user to confirm copyright, trademark, portrait rights, and brand compliance before commercial use.
- Do not claim that generated images are legally cleared for commercial use.
- Do not handle OCR, image recognition, table extraction, or screenshot diagnosis in this skill; image generation and image editing are the only supported tasks here.
- If generation fails or does not match expectations, offer to regenerate with refined prompt, style, ratio, or negative prompt.
