# File Upload Prompt Reference

## 使用场景

Use when the workflow needs the user to upload one or more files before another skill or the system parser can continue.

Trigger examples: 上传文件, 请上传文档, 需要简历文件, 上传合同, 上传 PDF, file upload, upload document, attach files.

## Payload 示例

```json
{
  "message": "我需要先拿到文件内容，上传后会继续处理。",
  "questions": [
    {
      "id": "source_files",
      "question": "请在聊天中上传需要处理的文件。支持 PDF、Word、TXT 或 Markdown；如果有多个版本，请一并上传并标明版本关系。"
    }
  ]
}
```

For resumes:

```json
{
  "message": "我需要先拿到简历内容，上传后会继续初筛。",
  "questions": [
    {
      "id": "resume_file",
      "question": "请上传候选人简历文件，支持 PDF、Word 或 TXT。若有岗位 JD，也可以同时粘贴或上传。"
    }
  ]
}
```

## 数据规则

- This skill only prompts the user to upload files through chat.
- Do not claim that this skill can parse or read uploaded files.
- After upload, rely on the system document parser or the appropriate downstream skill.
- State accepted file types in the visible question.
- If multiple files are needed, tell the user how to label them.
- Do not add unsupported fields such as `file_upload`, `accepted_file_types`, or `max_files`.

## 质询规则

Ask for upload only when the workflow cannot proceed from existing pasted text, chat context, or parsed document content. If parsed content is already available, route to the relevant business skill instead of asking for upload again.
