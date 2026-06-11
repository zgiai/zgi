---
name: resume-screening
description: Extract candidate information from existing resume text or parsed resume content and provide preliminary screening based on an optional JD or screening criteria. Use when the user asks for 简历初筛, 简历筛选, 候选人评估, 候选人摘要, 简历提取, 简历结构化, JD匹配, 岗位匹配, 面试问题, 是否进入下一轮, HR初筛, 人才库录入, resume screening, candidate screening, CV screening, JD matching, interview questions, or talent pool entry.
when_to_use: Use this skill only after resume content is already available from pasted text, chat context, or the system document parser. Do not use it to directly read, parse, extract, OCR, or inspect uploaded PDF, Word, TXT, scanned resume files, or other file bytes. If no JD is provided, only summarize the resume and extract general capabilities; do not produce a job-fit judgment. If the user asks to export the screening result as Word, PDF, Markdown, or another file, produce the screening content first and then route it to file-generator.
runtime_type: prompt
max_calls_per_turn: 5
timeout_seconds: 5
display:
  icon: user-check
  category: productivity
  label:
    en_US: Resume Screening
    zh_Hans: 简历初筛
  description:
    en_US: Extracts resume facts and gives preliminary JD-based screening when criteria are provided.
    zh_Hans: 提取简历关键信息，并在提供岗位要求时进行初步匹配判断。
  when_to_use:
    en_US: Use when existing resume content needs structured extraction, JD matching, or interview question suggestions.
    zh_Hans: 当已有简历内容需要结构化提取、岗位匹配或面试问题建议时使用。
  tags:
    en_US:
      - Resume
      - Screening
      - HR
    zh_Hans:
      - 简历
      - 初筛
      - 招聘
---

# Resume Screening Skill

Use this skill to extract candidate information from available resume content and produce preliminary screening analysis. This skill does not read files, parse file formats, OCR images, generate files, or make final hiring decisions.

## Scope

Use this skill for:

- HR resume pre-screening before batch review.
- Business interviewers quickly understanding a candidate's background.
- Preliminary JD matching when a job description or screening criteria are provided.
- Extracting structured resume information for talent pool entry.
- Identifying highlights, risks, unclear points, and interview question suggestions.

Do not use this skill to:

- Directly read, parse, extract, or inspect uploaded resume files or file bytes.
- Replace the system document parser.
- Make final hiring, rejection, compensation, title, or ranking decisions.
- Judge candidates based on gender, age, marital or childbearing status, ethnicity, religion, health status, disability, photo, household registration, or other sensitive attributes.
- Generate Word, PDF, Markdown, TXT, CSV, XLSX, or any downloadable file directly.
- Invent candidate facts, education, years of experience, skills, certificates, salary, city, availability, or work history not grounded in resume content.

## Routing Rules

- If resume content is already available from pasted text, chat context, or the system document parser, use this skill directly.
- If the user uploads a resume file and asks for screening, extraction, or JD matching, use this skill only after the system document parser has provided resume content.
- If the user only asks to read, parse, extract, or OCR a resume file, do not use this skill; that is a document parsing task.
- If the user says "read this resume and screen it", the system document parser must provide the source content first, then this skill performs screening.
- If the user asks to export the screening result, produce the screening content first, then route that content to `file-generator`.

## Workflow

1. Confirm that resume source content is available.
2. Identify whether a JD, job requirements, or screening criteria are available.
3. Choose the task type: resume summary, JD matching, criteria screening, interview questions, or talent pool entry.
4. If the task type is clear, read exactly one relevant reference before producing the result.
5. Extract candidate facts from source content only. Use `简历未体现` when a field is missing or unclear in Chinese output.
6. If a JD or criteria are provided, compare the resume against those requirements and separate matched evidence, gaps, risks, and interview-confirmation items.
7. If no JD is provided, do not output job-fit level, matching score, or enter-next-round recommendation. Output resume summary and general capability extraction only.
8. Avoid sensitive attributes entirely when making screening judgments or interview questions.
9. If the user requests export after the screening result is prepared, hand the prepared content to `file-generator`.

## Screening Judgment Rules

Use fit levels only when a JD or explicit screening criteria are available:

- `高`: Most core requirements have clear resume evidence, key experience or skills match, and risks are limited.
- `中`: Some core requirements match, but there are meaningful gaps, missing evidence, or points requiring interview confirmation.
- `低`: Key requirements are missing, experience direction is clearly mismatched, or the resume lacks enough evidence for the role.

Never output a final hiring or rejection decision. Use phrasing such as `建议进入下一轮：是/否/需补充确认` only as a preliminary screening recommendation grounded in provided JD or criteria.

## Sensitive Attribute Rules

- Do not use gender, age, marital or childbearing status, ethnicity, religion, health status, disability, photo, household registration, or similar sensitive attributes as screening evidence.
- Do not infer capability, stability, salary expectation, availability, or culture fit from sensitive attributes.
- Do not generate interview questions about sensitive attributes.
- If sensitive information appears in the resume, ignore it for screening or state that it is not used as an evaluation basis when necessary.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when a missing decision blocks reliable screening. Ask 1-4 focused questions. Options must be concrete answers that can be used directly. A single question may contain at most 5 options.

Ask when:

- No resume content is available.
- The user asks whether a candidate is suitable, but no JD or screening criteria are provided.
- Multiple resumes or multiple JDs exist and the mapping is unclear.
- The user asks for batch screening but ranking criteria or must-have requirements are unclear.
- Required screening conditions such as location, years, education, skills, or industry experience are not specified but materially affect the output.

Example `request_user_input` payload:

```json
{
  "message": "我可以做简历初筛，但需要先确认岗位和筛选重点。",
  "questions": [
    {
      "id": "screening_mode",
      "question": "这次初筛按什么方式进行？",
      "options": [
        { "label": "只提取简历摘要", "description": "不做岗位匹配判断。" },
        { "label": "按JD匹配", "description": "需要提供岗位JD或岗位要求。" },
        { "label": "按筛选条件", "description": "按学历、年限、城市、技能或行业经验筛选。" },
        { "label": "生成面试问题", "description": "基于简历亮点和疑问点生成问题。" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## Language Rules

- Match the user's requested output language. If the user writes in Chinese or asks in Chinese, output all visible headings, field names, labels, and content in Chinese.
- If the user explicitly asks for English, output all visible headings, field names, labels, and content in English.
- Do not mix Chinese and English section headings in the final answer. Avoid English labels such as `Summary`, `Keywords`, `Fit`, `Risk`, `Interview Questions`, `Next Round`, and `Notes` in Chinese output.
- For Chinese output, use Chinese labels such as `候选人摘要`, `结构化信息`, `核心经历`, `技能标签`, `匹配度判断`, `匹配依据`, `风险点`, `疑问点`, `面试建议问题`, and `是否建议进入下一轮`.
- Keep company names, school names, product names, certificates, technical terms, programming languages, tools, and quoted source terms in their original language when translation would change meaning.

## References

Read exactly one reference after choosing the task type:

| Requested task | Read reference |
| --- | --- |
| Resume summary, candidate summary, general capability extraction without JD | `resume-summary.md` |
| JD matching, role matching, whether to enter next round | `jd-match.md` |
| Screening by criteria such as education, years, city, skills, industry | `screening-criteria.md` |
| Interview questions, follow-up questions, risk probing | `interview-questions.md` |
| Talent pool entry, structured candidate record | `talent-pool-entry.md` |

If the user asks for multiple outputs, read the reference that matches the primary deliverable and include secondary sections only when they are directly supported by resume content and provided criteria.

## Constraints

- Do not expose internal reasoning or mention this skill.
- Do not output JSON unless the user explicitly asks for JSON.
- Do not directly read, parse, extract, OCR, or inspect uploaded resume files.
- Do not claim that files were parsed unless the system document parser already supplied parsed content.
- Do not make final hiring, rejection, compensation, title, or ranking decisions.
- Do not use sensitive attributes as screening evidence.
- If no JD or screening criteria are provided, do not produce job-fit level or next-round recommendation.
- Mark missing or unclear resume information as `简历未体现` in Chinese output.
- Every screening conclusion must be grounded in resume source text, parsed source content, JD text, or explicit user-provided criteria.
- Do not fabricate candidate experience, skills, education, certificates, projects, dates, responsibilities, or achievements.
- Do not generate files directly. Use `file-generator` only after the screening content is prepared and the user requested export.
