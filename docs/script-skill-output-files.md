# Script Skill 输出文件接入 ZGI

新建 Skill 时建议从
[`script-skill-development-guide.md`](script-skill-development-guide.md) 和其中的
[Python](examples/zgi-script-skill-template/) 或
[Node.js](examples/zgi-node-script-skill-template/) 可运行模板开始；本文只展开输出文件和前端附件链路。

本文面向 Script Skill 开发者和 ZGI 运行时开发者，说明如何把脚本生成的文件接入对话附件链路，使文件可以在消息中展示、下载，并在刷新会话后继续访问。

## 先明确：文本路径不是附件

下面这样的模型回答只是在展示字符串：

```text
结果包：artifacts/template-analysis-results.zip
报告：artifacts/template-analysis-report.md
```

浏览器无法访问这个路径。`artifacts/` 位于一次性 Sandbox 内部，Sandbox 在 Skill 调用结束后会被删除。ZGI 也不会解析回答中的 Markdown 或代码文本来猜测附件。

正确接入后，每个文件都会经过以下链路：

```text
Skill 写入 artifacts/
  -> Sandbox 返回 artifact_manifests
  -> API 在清理 Sandbox 前下载文件并持久化为 tool file
  -> Chat Runtime 产生 file tool message 和 skill_artifact_created 事件
  -> 消息 metadata.generated_files 保存文件身份
  -> Web 显示文件卡片，并使用 ZGI URL 预览或下载
```

如果 Skill 通过 ZGI 内置的 `run_script` 执行，Skill 开发者只需要完成“声明产物”和“写入文件”两步；后面的持久化、事件和前端展示由 ZGI 完成。Skill 脚本不应直接调用 Sandbox 下载接口，也不需要自行构造 `skill_artifact_created` 事件。

## Skill 侧接入

### 1. 声明允许收集的产物

在 Skill 包根目录创建 `skill.manifest.json`。推荐显式声明输出协议，不要依赖无 manifest 包的兼容扫描逻辑。

```json
{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 300000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 2097152,
  "result_mode": "mixed"
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `allowed_artifact_paths` | 只允许 `artifacts` 或其子路径，例如 `artifacts/reports`。 |
| `max_artifact_count` | 本次调用最多收集的文件数。普通 Script Skill 的默认上限是 10。 |
| `max_artifact_bytes` | 本次产物清单允许的总字节数。普通 Script Skill 的默认上限是 2 MiB。 |
| `result_mode` | 同时返回 stdout 结果和文件时使用 `mixed`；只产出文件可使用 `artifacts`。 |

四种模式的推荐用法如下：

| 模式 | stdout 契约 | 产物契约 |
| --- | --- | --- |
| `stdout_json` | 输出一个 JSON object，模型收到 JSON Tool 消息。 | 可不生成文件。 |
| `stdout_text` | 输出 UTF-8 文本，模型收到 text Tool 消息。 | 可不生成文件。 |
| `artifacts` | stdout 可留空。 | 文件是主要交付物。 |
| `mixed` | 输出 JSON object 或文本摘要。 | 同时返回一个或多个文件。 |

stderr 始终作为诊断 log 消息返回，不是业务结果通道。合法 JSON 数组或标量应放入
JSON object 的字段中，避免被作为普通文本传递给模型。

由服务端运行时授权的包可以获得更高限额，当前 API 管理授权上限为 20 个文件和 256 MiB，部署侧策略还可以进一步收紧。Skill 包不能仅靠提高 manifest 数值来获得该权限。不要在包内指定受管 `dependency_profile`；依赖环境仍由 ZGI 的依赖构建与授权流程决定。

### 2. 把最终文件写到 `artifacts/`

文件必须在脚本退出前写完、关闭，并且是普通文件。下面的示例会生成截图中提到的报告和结果包：

```python
import json
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


def main() -> int:
    artifact_dir = Path("artifacts")
    artifact_dir.mkdir(parents=True, exist_ok=True)

    report_path = artifact_dir / "template-analysis-report.md"
    report_path.write_text("# 分析报告\n\n这里是报告内容。\n", encoding="utf-8")

    archive_path = artifact_dir / "template-analysis-results.zip"
    with ZipFile(archive_path, "w", compression=ZIP_DEFLATED) as archive:
        archive.write(report_path, arcname=report_path.name)

    # stdout 只描述执行结果。附件注册以 Sandbox 的 artifact_manifests 为准。
    print(json.dumps({
        "status": "success",
        "summary": "已生成分析报告和结果包",
        "artifact_count": 2
    }, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

注意：

- 路径相对于 Skill 包的执行工作目录，不要写到 `/tmp`、`outputs/` 或用户目录。
- 不要创建指向其他位置的符号链接；产物扫描和下载不接受路径逃逸。
- 使用能表达真实格式的扩展名。ZGI 会结合扩展名、Sandbox 的 `content_type` 和文件内容确定 MIME 类型。
- stdout 中可以报告文件数量和用途，但不要把 `artifacts/...` 描述为可下载 URL。
- 脚本非零退出前生成的诊断文件也可能被收集，但文件存在不代表这次调用成功。

## ZGI 后端接入契约

本仓库内置的 `run_script` 已实现这段桥接。新增执行器或自定义 Tool 时，也必须提供等价行为，不能把 Sandbox 本地路径直接返回给前端。

### 1. 从 Sandbox 获取产物清单

`POST /v1/exec/skill` 的 `data.artifact_manifests` 是文件发现的事实来源。每个清单项至少包含路径、大小和内容类型，例如：

```json
{
  "path": "artifacts",
  "file_count": 2,
  "items": [
    {
      "path": "artifacts/template-analysis-results.zip",
      "size": 12345,
      "content_type": "application/zip"
    },
    {
      "path": "artifacts/template-analysis-report.md",
      "size": 2048,
      "content_type": "text/markdown"
    }
  ]
}
```

后端必须再次校验路径位于 manifest 允许的目录中，并遵守文件数量和字节限制。

### 2. 在 Sandbox 销毁前持久化

对每个清单项执行以下操作：

1. 使用当前 `sandbox_id` 和原始相对路径调用 Sandbox 文件下载接口，并请求 Base64 编码。
2. 解码并校验真实字节数。
3. 使用当前 `organization_id`、`user_id` 和可用的 `conversation_id` 创建 ZGI tool file。
4. 生成 ZGI 签名访问 URL 和下载 URL。
5. 删除 Sandbox 前完成所有文件的持久化尝试；无论持久化成功还是失败，调用结束时都应清理 Sandbox。

不要把 `/v1/files/download?sandbox_id=...` 暴露给浏览器。该地址属于内部服务接口，而且 Sandbox 生命周期结束后文件就不存在了。

持久化失败时，产物结果应明确包含 `persisted: false` 以及稳定原因。当前实现使用的常见原因包括：

- `size_limit_exceeded`
- `download_failed`
- `unsupported_encoding`
- `decode_failed`
- `persist_failed`

### 3. 转换成 file tool message

每个成功持久化的产物必须产生一条 `type: "file"` 的 Tool 消息。文件对象至少需要包含：

```json
{
  "type": "file",
  "text": "https://zgi.example/files/signed-id?download=1",
  "meta": {
    "file": {
      "id": "tool-file-id",
      "filename": "template-analysis-results.zip",
      "extension": ".zip",
      "mime_type": "application/zip",
      "size": 12345,
      "url": "https://zgi.example/files/signed-id",
      "download_url": "https://zgi.example/files/signed-id?download=1",
      "transfer_method": "tool_file"
    }
  }
}
```

仅在 JSON 结果里返回 `path` 或 `download_url` 不够；Chat Runtime 是从 `type: "file"` 且 `meta.file` 有效的 Tool 消息中提取附件的。

### 4. 产生对话事件并保存历史元数据

Chat Runtime 会把 file tool message 规范化为 `skill_artifact_created` SSE 事件。当前标准事件是扁平结构：

```text
event: skill_artifact_created
data: {
  "conversation_id": "conversation-id",
  "message_id": "assistant-message-id",
  "invocation_id": "tool-call-id",
  "skill_id": "skill-id",
  "tool_name": "run_script",
  "artifact_id": "tool_file:tool-file-id",
  "artifact_type": "file",
  "file_id": "tool-file-id",
  "tool_file_id": "tool-file-id",
  "filename": "template-analysis-results.zip",
  "extension": ".zip",
  "mime_type": "application/zip",
  "size": 12345,
  "url": "https://zgi.example/files/signed-id",
  "download_url": "https://zgi.example/files/signed-id?download=1",
  "transfer_method": "tool_file",
  "created_at": 1789430400
}
```

同一个文件身份还必须写入助手消息的 `metadata.generated_files`，保证刷新页面或重新打开会话后仍能恢复文件卡片。tool file 的签名 URL 不适合长期原样存储；读取历史消息时应根据 `tool_file_id` 重新签名并补全 `url`、`download_url` 和可用状态。

`skill_artifact_created` 表示“对话中产生了一个文件”。如果用户随后明确要求把文件保存到文件管理，应再调用文件管理 Skill 的 `save_file_to_management`，并保留源 `tool_file_id` 与目标 `upload_file_id` 的关系；不要把生成附件和保存到文件管理混成同一个动作。

## Web 接入要求

当前 Web 已支持 `skill_artifact_created`。事件进入 reducer 后会写入对应消息的 `metadata.generated_files`，消息组件使用 `download_url || url` 渲染下载入口。

前端会拒绝缺少关键字段的事件。下列字段必须有效：

- `conversation_id`、`message_id`
- `skill_id`、`tool_name`
- `file_id`，或可推导它的 `tool_file_id` / `upload_file_id`
- `filename`、`extension`、`mime_type`
- 数字类型的 `size`
- 普通 tool file 的 `url`

因此，不应在前端增加“扫描助手正文中的 `artifacts/...` 并生成链接”的兼容逻辑。这会绕过租户权限、签名 URL、生命周期和历史恢复机制。

## 针对“正文有路径但没有文件卡片”的排查

按以下顺序检查，最容易定位断点：

| 观察结果 | 主要原因 | 处理方式 |
| --- | --- | --- |
| Sandbox 中没有目标文件 | 脚本写到了错误目录，或写入尚未关闭 | 确保脚本退出前把普通文件写入 `artifacts/`。 |
| 文件存在，但 `artifact_manifests` 没有条目 | `allowed_artifact_paths` 不匹配，数量/总字节超限，或没有走 `/v1/exec/skill` 协议 | 修正 manifest，并检查 Sandbox 返回的限制错误。 |
| 清单有条目，但产物是 `persisted: false` | 下载、解码、大小校验或 tool file 落库失败 | 查看产物的 `reason` 和 `error`；重点核对 API 执行上下文中的 organization/user 身份和文件服务配置。 |
| Tool 结果只有 JSON，没有 `type: "file"` | 执行器漏掉了产物持久化或 file message 转换 | 复用 `SandboxScriptRunner.prepareArtifacts` 和 `skillScriptMessages` 的现有模式。 |
| 有 file tool message，但没有 SSE 文件事件 | Chat Runtime 没有从 Tool 消息提取或发送附件事件 | 检查 `SkillArtifactsFromToolMessages` 以及成功、失败两条事件发送分支。 |
| 收到事件，但前端没有文件卡片 | 事件缺少 `skill_id`、`tool_name`、文件身份、文件名、扩展名、MIME、数字大小或 URL | 对照上一节补全字段，并确认浏览器实际收到的数据类型。 |
| 当前消息可下载，刷新后消失 | `metadata.generated_files` 未保存或历史读取未恢复 | 记录稳定的 `tool_file_id`，在历史响应中重新生成签名 URL。 |
| Sandbox 删除后链接失效 | 暴露了 Sandbox 内部下载地址 | 在清理 Sandbox 前复制到 ZGI tool file，只向前端返回 ZGI URL。 |

对于截图中的情况，首先在同一次调用的 Tool 结果中搜索：

```json
{
  "artifacts": [
    {
      "path": "artifacts/template-analysis-results.zip",
      "persisted": true,
      "file_id": "...",
      "download_url": "..."
    }
  ]
}
```

如果只有模型正文中的两个路径，没有上面这段结构化产物信息，问题位于 Skill 输出目录或 Sandbox 产物发现阶段；如果结构化信息完整但 UI 不显示，问题位于 Chat Runtime 事件或 Web 字段契约阶段。

## 验收清单

一次完整的接入测试至少覆盖：

1. Skill 同时生成 `.zip` 和 `.md`，Sandbox 的 `artifact_manifests` 精确列出两个文件。
2. API 在 Sandbox 删除前将两个文件持久化，Tool 结果中有两条 `type: "file"` 消息。
3. SSE 流发送两个 `skill_artifact_created` 事件，字段满足 Web 的必填约束。
4. 助手消息历史中 `metadata.generated_files` 包含两个稳定文件身份。
5. 当前页面和刷新后的页面都展示两个文件卡片。
6. 下载内容与 Sandbox 原文件的字节数和 SHA-256 一致。
7. 删除 Sandbox 后，ZGI 下载链接仍可用。
8. 分别验证超限文件、持久化失败和脚本非零退出，确保错误状态不会被文件产出错误地标记为成功。

## 相关实现

- API 的产物发现与持久化：`api/internal/modules/skills/sandbox_script_runner_artifacts.go`
- API 的 file tool message 转换：`api/internal/modules/skills/sandbox_script_runner_messages.go`
- Chat Runtime 的附件事件规范化：`api/internal/capabilities/chatruntime/skilltrace/payloads.go`
- Web 的事件校验与状态写入：`web/src/components/chat/controllers/aichat/reducers/files.ts`
- Web 的文件字段类型：`web/src/services/types/aichat.ts`
- Sandbox 的产物清单协议：`sandbox/.kest/sandbox-archive-skill-artifacts.flow.md`
- Script Skill 输入文件协议：`docs/script-skill-input-files.md`
