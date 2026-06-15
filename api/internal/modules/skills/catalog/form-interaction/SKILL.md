---
name: form-interaction
description: Design structured user clarification flows with the existing request_user_input capability so the assistant asks stable, focused questions with usable quick options instead of plain natural-language follow-up. Use when the user or another skill needs 问答交互, 表单交互, 结构化质询, 弹出输入框, 选项框, 单选, 多选, 补充信息, 确认参数, 文件上传提示, 表单填写, questionnaire, form interaction, structured clarification, user input, quick replies, or parameter confirmation.
when_to_use: Use this skill when a workflow cannot proceed reliably until the user provides missing information, chooses options, confirms parameters, supplies free-text details, or uploads a file through the normal chat upload flow. This skill only designs and triggers structured request_user_input prompts using the currently supported message/questions/options contract; it does not add new frontend components, render custom forms, process submitted files, execute business logic, or generate final business outputs.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: panel-top-open
  category: productivity
  label:
    en_US: Form Interaction
    zh_Hans: 表单交互
  description:
    en_US: Creates structured clarification prompts with focused questions and concrete quick options.
    zh_Hans: 使用结构化问题和可直接使用的选项收集用户补充信息。
  when_to_use:
    en_US: Use when a workflow needs missing parameters or user confirmation before continuing.
    zh_Hans: 当流程继续前需要用户补充参数、选择选项或确认信息时使用。
  tags:
    en_US:
      - Form
      - Clarification
      - Input
    zh_Hans:
      - 表单
      - 质询
      - 输入
---

# Form Interaction Skill

Use this skill to create structured user clarification flows with the existing `request_user_input` capability. It helps other skills ask focused questions, show concrete quick options, and pause cleanly until the user answers.

This skill does not change frontend behavior. It must use only the currently supported `request_user_input` contract: `message`, `questions`, `id`, `question`, `options`, `label`, and `description`. In short, use the current message/questions/options shape only.

## Scope

Use this skill for:

- Asking for missing parameters before a business skill or tool can proceed.
- Confirming user choices such as output format, chart type, tone, comparison dimension, screening mode, or generation style.
- Turning vague follow-up questions into structured question cards.
- Designing single-choice style questions with mutually exclusive quick options.
- Designing multi-choice style questions by asking the user to select one or more labels from a short option list.
- Asking for free-text details by omitting `options`.
- Asking the user to upload files through the normal chat upload flow before another skill consumes parsed content.
- Planning multi-step clarification where the next step depends on the user's answer.

Do not use this skill to:

- Modify `request_user_input` or depend on unsupported frontend fields.
- Promise native date pickers, file upload widgets, checkboxes, custom form layouts, conditional UI rendering, or validation components.
- Execute business logic, parse files, generate charts, generate images, write summaries, screen resumes, compare documents, or create files.
- Ask every possible question at once when only a few blocking details are needed.
- Use ordinary Markdown follow-up questions when a structured `request_user_input` call is needed.

## Supported Interaction Shapes

The current contract supports these reliable interaction shapes:

- Open text input: ask a question and omit `options`.
- Single-choice style quick replies: provide 2-5 concrete, mutually exclusive `options`.
- Multi-choice style selection: provide 2-5 option labels and explicitly ask the user to choose one or more labels.
- Confirmation: provide concrete options such as `确认` and `需要修改`.
- Date or number collection: ask as open text and state the expected format in the visible question.
- File upload prompt: ask the user to upload the file in chat, and state accepted file types in the visible question.

Do not add unsupported fields such as `type`, `required`, `placeholder`, `validation`, `accepted_file_types`, `max_files`, `date_picker`, or `component`. They may be ignored by the current frontend and would make behavior unreliable.

## Workflow

1. Identify the workflow that is blocked and the smallest set of information needed to continue.
2. Choose the interaction shape: open text, quick single choice, multi-choice instruction, confirmation, date/number as text, or file upload prompt.
3. Read exactly one relevant reference before calling `request_user_input`.
4. Build a concise `message` that explains why input is needed and what will happen next.
5. Provide 1-4 focused questions. Prefer 1-3 questions unless the workflow truly requires more.
6. Use `options` only when each option is a concrete answer that can be used directly.
7. After calling `request_user_input`, stop the turn and wait for the user's answer.
8. When the user answers, route back to the original business skill or continue the workflow using the submitted information.

## Clarification Rules

Call `request_user_input` instead of writing a plain clarification when:

- Missing information blocks reliable progress.
- The user gave a generic request such as "生成图表", "帮我做个对比", "整理一下", or "按岗位筛简历" and a key parameter is missing.
- The user must choose from a small set of valid options.
- A tool call would fail or produce a materially wrong result without the missing parameter.
- The user needs to confirm a risky, costly, irreversible, external-facing, or policy-sensitive choice.
- The workflow needs a file, but no file or parsed file content is available.

Do not ask when:

- The answer is already provided in the current conversation.
- A safe default is clearly documented by the active business skill and does not materially change the result.
- The user explicitly says not to ask and the task can be completed with transparent assumptions.
- Asking would only collect optional preferences that are not needed to proceed.

## Payload Rules

Use this shape:

```json
{
  "message": "我需要先确认几个关键信息，确认后会继续处理。",
  "questions": [
    {
      "id": "output_format",
      "question": "希望输出成哪种形式？",
      "options": [
        { "label": "摘要", "description": "先给核心结论和重点。" },
        { "label": "表格", "description": "按维度整理，便于对比。" },
        { "label": "详细清单", "description": "逐项列出依据和差异。" }
      ]
    }
  ]
}
```

Rules:

- `message` is required and should be brief.
- `questions` is required and must contain 1-5 questions.
- Prefer 1-3 questions.
- Each question should have a stable `id` using lowercase letters, numbers, and underscores.
- A question may omit `options` for free-text input.
- A question with `options` must use no more than 5 options.
- Do not include vague quick options such as `其他`, `随便`, `都可以`, `不确定`, `看情况`, or `自由发挥`.
- If more than 5 options are needed, ask an open-text question or split the flow into multiple rounds.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese `message`, `question`, `label`, and `description`.
- Do not mix Chinese and English structural labels in user-visible text.
- Keep tool names, skill names, product names, code identifiers, file formats, and exact source terms in their original language when translation would change meaning.

## References

Read exactly one reference after identifying the interaction need:

| Interaction need | Read reference |
| --- | --- |
| Open text, date as text, number as text, file upload prompt, confirmation | `question-shapes.md` |
| Single-choice and multi-choice quick options | `option-rules.md` |
| Required fields, format hints, safe validation wording | `validation-guidance.md` |
| Multi-round clarification and returning to the original workflow | `multi-step-flow.md` |
| Asking the user to upload files without parsing them in this skill | `file-upload-prompt.md` |

If multiple references could apply, choose the one that matches the main blocker.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not modify or extend `request_user_input`.
- Do not include unsupported request fields such as `type`, `component`, `placeholder`, `validation`, `accepted_file_types`, or `max_files`.
- Do not promise fixed frontend widgets beyond the existing structured question card and quick options.
- Do not execute the downstream business task inside this skill.
- Do not call downstream tools in the same turn after `request_user_input`.
- Do not ask for sensitive personal data unless it is necessary for the user's requested workflow.
- Do not ask for passwords, secrets, private keys, tokens, or full payment card data.
- Do not generate files directly. Use the downstream business skill first, then `file-generator` only if the user requested export.
