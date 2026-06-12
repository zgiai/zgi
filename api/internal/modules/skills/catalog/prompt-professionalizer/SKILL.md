---
name: prompt-professionalizer
description: Professionalize vague, casual, incomplete, or tool-specific prompts into structured high-quality prompts for image generation, animation/video generation, technical architecture diagrams, data visualization, chart generation, and general prompt optimization. Use when the user asks for 提示词优化, Prompt 优化, 专业提示词, 生图提示词, 图片生成提示词, 动画提示词, 视频提示词, 架构图提示词, 技术架构图, 数据可视化提示词, 图表提示词, 帮我改成更专业的提示词, 帮我优化一下给某工具用, optimize prompt, professional prompt, image prompt, video prompt, architecture diagram prompt, data visualization prompt, chart prompt, or prompt template.
when_to_use: Use this skill when the user wants to turn an idea, rough description, existing prompt, or incomplete generation request into a professional prompt for a downstream tool. This skill is also the required preflight step before professional generation tools such as image-generator, architecture-diagram-generator, chart-generator, animation/video generators, and data visualization tools when the user's request is casual, vague, incomplete, or not yet structured for that tool. This skill only produces prompts, parameter suggestions, negative prompts, and structured instructions; it does not directly generate images, videos, architecture diagrams, charts, dashboards, files, or other artifacts. If the user asks to run a downstream generation tool, first prepare the professional prompt, then route the prompt to the corresponding skill or tool only when explicitly requested and available.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: wand-sparkles
  category: productivity
  label:
    en_US: Prompt Professionalizer
    zh_Hans: 提示词专业化
  description:
    en_US: Turns rough requests into professional prompts for image, video, architecture diagram, and visualization tools.
    zh_Hans: 将口语化需求整理为适合生图、视频、架构图和可视化工具的专业提示词。
  when_to_use:
    en_US: Use when a prompt needs clearer structure, missing-parameter checks, or downstream tool adaptation.
    zh_Hans: 当提示词需要结构化、补齐关键参数或适配下游工具时使用。
  tags:
    en_US:
      - Prompt
      - Optimization
      - Generation
    zh_Hans:
      - 提示词
      - 优化
      - 生成
---

# Prompt Professionalizer Skill

Use this skill to turn casual, vague, incomplete, or tool-specific generation requests into professional prompts that downstream tools can understand. This skill prepares prompts only; it does not directly invoke image generation, video generation, architecture diagram generation, data visualization, chart generation, or file generation tools.

## Preflight Requirement

Use `prompt-professionalizer` before calling professional generation tools when the user request is oral, vague, incomplete, or not already shaped for the target tool. This preflight applies to `image-generator`, `architecture-diagram-generator`, `chart-generator`, animation/video generation tools, data visualization tools, and similar professional generators.

Skip this preflight only when the user already provides a complete structured prompt or complete tool payload with the required target type, content, style or rendering choices, and key parameters for the downstream tool.

## Scope

Use this skill for:

- Image-generation prompts for tools such as Midjourney, Stable Diffusion, DALL-E, 即梦, 可灵, 通义万相, or similar tools.
- Animation or video-generation prompts for tools such as 可灵, Runway, Pika, 即梦视频, Sora-style tools, or similar tools.
- Technical architecture diagram prompts for Mermaid, PlantUML, Draw.io, Excalidraw, C4 diagrams, deployment diagrams, sequence diagrams, flowcharts, and ER diagrams.
- Data visualization prompts for ECharts, Tableau, Power BI, Datawrapper, chart generators, dashboards, and BI tools.
- General prompt optimization when the user asks to make a prompt more professional, precise, structured, controllable, or suitable for a specified tool.

Do not use this skill to:

- Directly generate images, videos, charts, architecture diagrams, dashboards, files, or code.
- Invent business facts, technical modules, data fields, metrics, dimensions, brands, people, or constraints not provided by the user.
- Change the user's original intent or silently add a new goal.
- Produce prompts for illegal, infringing, pornographic, graphic violent, deceptive identity, or clearly harmful outputs.

## Workflow

1. Identify the target prompt type: image generation, animation/video generation, architecture diagram, data visualization, general prompt optimization, or unknown.
2. Check whether key information is missing for that target type.
3. If critical information is missing and the user did not say "do not ask", call `request_user_input` before producing the final professional prompt.
4. Read exactly one reference for the selected prompt type.
5. Combine the user's original request, any provided target tool, and any clarified answers into a professional prompt.
6. Output a structured result with `专业化 Prompt`, `参数建议`, optional `负面 Prompt`, and `说明`.
7. If the user explicitly asks to run a downstream generation tool after prompt preparation, route the prepared prompt to the corresponding skill or tool only when that tool is available and the request is explicit.
8. When another professional generation skill invokes this skill as preflight, return the optimized prompt and structured parameters that the downstream skill should use; do not produce the final artifact inside this skill.

## Intent Detection

- Image generation: user asks for image, poster, cover, illustration, product scene, visual, logo draft, banner, 生图, 图片生成, 画面, 海报, 插画.
- Animation/video generation: user asks for video, animation, shot, camera motion, scene transition, 可灵, Runway, Pika, Sora, 视频, 动画, 镜头.
- Architecture diagram: user asks for architecture, system diagram, flowchart, deployment, sequence diagram, ER diagram, Mermaid, PlantUML, 架构图, 技术架构图, 流程图, 时序图.
- Data visualization: user asks for chart, dashboard, visualization, ECharts, Tableau, Power BI, 图表, 可视化, 数据看板.
- General optimization: user asks to optimize, professionalize, rewrite, improve, structure, adapt to a tool, 提示词优化, 更专业.

If the target type cannot be determined, call `request_user_input`.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when missing information would materially reduce prompt quality. Ask at most 3 questions in one turn. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Must ask when:

- The target prompt type or target tool is unknown.
- Image prompt lacks a subject or style.
- Video prompt lacks action or duration.
- Architecture diagram prompt lacks system modules or diagram type.
- Data visualization prompt lacks data fields or analysis goal.
- The user only says something like "make this more professional" without target tool or use case.
- The request is too short, such as "generate a tech image".

Do not ask when:

- The user explicitly says not to ask and to optimize directly.
- The prompt is already complete enough.
- The missing detail can be filled with a reasonable default and will not materially change the result.
- The user only wants a reusable prompt template.

When using defaults, label them under `默认假设`.

Example `request_user_input` payload:

```json
{
  "message": "我可以把需求整理成专业提示词，但需要先确认关键参数。",
  "questions": [
    {
      "id": "target_type",
      "question": "这个提示词主要给哪类工具使用？",
      "options": [
        { "label": "生图工具", "description": "用于图片、海报、插画、封面生成。" },
        { "label": "视频工具", "description": "用于动画、视频、镜头生成。" },
        { "label": "架构图工具", "description": "用于 Mermaid、PlantUML、Draw.io 等。" },
        { "label": "数据可视化", "description": "用于图表、看板、BI 工具。" },
        { "label": "通用优化", "description": "只提升表达、结构和可控性。" }
      ]
    },
    {
      "id": "primary_goal",
      "question": "最重要的生成目标是什么？"
    },
    {
      "id": "output_language",
      "question": "希望输出哪种语言的提示词？",
      "options": [
        { "label": "中文", "description": "输出中文专业提示词。" },
        { "label": "英文", "description": "输出英文专业提示词。" },
        { "label": "中英双版", "description": "同时输出中文和英文版本。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## Output Rules

Default Chinese output:

```md
## 专业化 Prompt

...

## 参数建议

- 工具类型：
- 风格：
- 画幅/尺寸：
- 输出形式：

## 负面 Prompt（可选）

...

## 说明

...
```

For architecture diagrams, prefer:

```md
## 专业化 Prompt

请生成一张【图类型】，用于展示...

## 图表结构

- 用户层：
- 应用层：
- 服务层：
- 数据层：
- 外部系统：

## 关系说明

...

## 推荐图类型

Mermaid flowchart / C4 架构图 / 部署图
```

For data visualization, prefer:

```md
## 专业化 Prompt

请基于以下数据生成一张...

## 图表配置建议

- 图表类型：
- X 轴：
- Y 轴：
- 分组字段：
- 筛选条件：
- 排序方式：
- 标题：
```

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- If the user asks for both Chinese and English, keep each version internally consistent.
- Do not mix Chinese and English section headings in a single-language output.
- Keep target tool names, model names, product names, system names, code identifiers, data field names, and quoted source terms in their original language when translation would change meaning.

## References

Read exactly one reference after identifying the target prompt type:

| Target prompt type | Read reference |
| --- | --- |
| Image generation, poster, cover, illustration, product scene, visual design | `image-prompt.md` |
| Animation, video generation, shot, camera movement, transition | `video-prompt.md` |
| Technical architecture diagram, Mermaid, PlantUML, C4, deployment, flowchart, sequence, ER | `architecture-diagram-prompt.md` |
| Data visualization, chart generation, dashboard, ECharts, Tableau, Power BI | `data-visualization-prompt.md` |
| General prompt optimization, rewrite, structure, tool adaptation | `general-prompt-optimization.md` |
| Missing information, clarification, defaults, direct optimization rules | `clarification-rules.md` |

If the request combines multiple prompt types, read the reference for the primary downstream tool and include secondary guidance only when it does not conflict.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not directly call downstream generation tools unless the user explicitly asks after the prompt is prepared and the target tool is available.
- Before professional generation tools, use this skill as the required preflight when the request is vague, casual, incomplete, or not already structured for the target tool.
- Do not invent facts, data fields, metrics, dimensions, technical modules, deployment layers, user roles, actions, camera movements, or tool parameters not provided by the user.
- If defaults are used, mark them under `默认假设`.
- Do not create prompts for illegal, infringing, pornographic, graphic violent, deceptive identity, or clearly harmful outputs.
- Do not simply polish wording; convert the request into a structured, professional prompt that a downstream tool can use.
- If the target tool has a known special format requirement from the user's request, adapt the prompt to that format.
