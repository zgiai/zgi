# ZGI Script Skill 开发规范

本文定义自定义 Script Skill 的目录、参数、文件、依赖和输出契约。建议直接复制
以下可运行示例，再删除业务不需要的字段：

- [Python 完整示例](examples/zgi-script-skill-template/)
- [Node.js 完整示例](examples/zgi-node-script-skill-template/)

## 能力边界

当前运行时适合单次调用内完成的文本、数据和文件处理任务。脚本通过 JSON 接收
参数，读取 ZGI 已授权的输入文件，通过 stdout 返回结果，并把下载文件写入
`artifacts/`。

| 场景 | 支持情况 | 开发要求 |
| --- | --- | --- |
| Prompt-only Skill | 支持 | 只需要 `SKILL.md`。 |
| Python Script | 支持 | 使用 `scripts/run.py` 或显式 manifest 入口。 |
| Node.js Script | 支持 | manifest 入口必须是 `scripts/*.js`。 |
| 本地模块 | 支持 | 放在 `scripts/` 下，可使用 `bundled_packages` 声明。 |
| 第三方 Python/Node 包 | 支持 | 精确声明并由部署侧 Profile 提前准备。 |
| 输入文件和下载产物 | 支持 | 使用 manifest 声明，脚本不得持有文件服务凭据。 |
| 多次调用保留 Sandbox 状态 | 不支持 | 一次调用完成流程，或用产物作为下一次输入。 |
| 运行时安装 pip、npm、apt 包 | 不支持 | 使用预构建依赖 Profile。 |
| 任意公网访问 | 默认不支持 | 使用经过授权和审计的 ZGI Connector/Tool。 |
| Shell、R、Java 直接入口 | 不支持 | 通过 Python/Node 包装，且二进制必须在 Profile 中。 |
| 常驻进程、监听端口、后台任务 | 不支持 | 脚本必须在 manifest 超时内退出。 |

## 标准目录

```text
your-skill/
├── SKILL.md
├── skill.manifest.json
├── requirements.txt              # 可选；Python 第三方依赖
├── package.json                  # 可选；Node.js 第三方依赖
├── scripts/
│   ├── run.py                    # Python 入口，或 run.js
│   └── your_local_package/       # 随 Skill 打包的本地模块
├── references/                   # 可选；按需读取的详细说明
└── examples/                     # 可选；本地冒烟输入
```

结构限制：

1. ZIP 根目录或唯一顶层目录中必须存在 `SKILL.md`。
2. Skill 名称只能使用小写字母、数字和连字符。
3. 自定义 Skill 使用 `runtime_type: prompt`，不得声明任意 provider 或普通 Tool。
4. 脚本入口及本地代码必须在 `scripts/` 下。
5. 普通包解压后不超过 64 MiB、600 个文件，单文件不超过 10 MiB。
6. 不要提交虚拟环境、`node_modules`、缓存、日志、上传文件、生成产物或密钥。

## `SKILL.md` 前置元数据

```yaml
---
name: your-skill
description: 说明 Skill 做什么，以及什么请求应使用它。
runtime_type: prompt
timeout_seconds: 30
script_input_schema:
  type: object
  additionalProperties: false
  required: [input]
  properties:
    input:
      type: string
      minLength: 1
---
```

| 字段 | 默认值 | 作用与限制 |
| --- | --- | --- |
| `name` | 无，必填 | Skill 稳定标识；只能使用小写字母、数字和连字符。 |
| `description` | 无，必填 | 告诉模型 Skill 做什么、何时应选择它。 |
| `when_to_use` | 自定义 Skill 回退为 `description` | 更明确的选择条件；建议在边界复杂时填写。 |
| `runtime_type` | 无 Tool 时归一化为 `prompt` | 自定义 Skill 必须使用 `prompt`。 |
| `max_calls_per_turn` | `20` | 单轮调用上限；示例正文进一步要求只调用 `run_script` 一次。 |
| `timeout_seconds` | 有脚本时 `30` | Skill 默认超时；manifest 的 `timeout_ms` 是脚本最终执行限制。 |
| `script_input_schema` | 默认 `run_script` 参数 | 可选的完整参数 Schema；有值时必须存在 `scripts/`，且根节点必须是 object。 |
| `display` | 从基础字段回退 | 可选展示元数据，支持 `icon`、`category`、`scenarios`、多语言 `label`、`description`、`when_to_use` 和 `tags`。 |
| `supported_callers` | `aichat`、`agent`、`digital_human` | 可选调用面白名单；还支持 `workflow`。未知值会被忽略。 |
| `required_config` | 空 | 可选配置前置条件列表；值会转为小写、去重和排序。 |
| `connector_requirements` | 空 | 可选 Connector 前置条件；每项包含 `connector_id`、`operation_ids`、`required`，不会赋予脚本网络权限。 |
| `provider_type`、`provider_id`、`tools`、`tool_governance` | 空 | 系统 Tool Skill 字段；自定义 Script Skill 不得声明普通 Tool。 |

正文应说明何时调用 `run_script`、成功结果的含义、文件处理方式和必须拒绝或转交
其他 Tool 的操作。详细说明放入 `references/`，并从正文标明何时读取。

## 输入参数类型

`script_input_schema` 最大 128 KiB、最多 32 层，只允许 `#` 开头的本地 `$ref`，
禁止 `$id` 和外部引用。推荐始终设置 `additionalProperties: false`。

| JSON 类型 | YAML 示例 | 常用限制 |
| --- | --- | --- |
| string | `type: string` | `minLength`、`maxLength`、`pattern`、`format`、`enum` |
| integer | `type: integer` | `minimum`、`maximum`、`multipleOf` |
| number | `type: number` | `minimum`、`maximum`、`exclusiveMinimum` |
| boolean | `type: boolean` | 通常无需额外限制 |
| array | `type: array` | `items`、`minItems`、`maxItems`、`uniqueItems` |
| object | `type: object` | `properties`、`required`、`additionalProperties` |
| null | `type: [string, "null"]` | 用联合类型表达可空值 |

可以使用 `enum`、`const`、`oneOf`、`anyOf`、`allOf` 和本地 `$defs`，但应保持
参数结构对模型可理解。不要接受任意 Shell 字符串、解释器、绝对路径、环境变量
或用户指定工作目录。

## `skill.manifest.json`

```json
{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "bundled_packages": [
    {"name": "your_local_package", "path": "scripts/your_local_package"}
  ],
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 3,
  "max_artifact_bytes": 2097152,
  "result_mode": "mixed"
}
```

| 字段 | 默认值 | 作用与限制 |
| --- | --- | --- |
| `entrypoint` | `scripts/run.py` | 包内入口，必须位于 `scripts/`。Python 使用 `.py`，Node.js 使用 `.js`。 |
| `language` | `python3` | `python3` 或 `nodejs`。 |
| `bundled_packages` | 空 | 可选本地模块清单，只做识别和校验，不安装包、不修改模块搜索路径。 |
| `dependencies` | 空 | 可选第三方依赖请求，支持 `python`、`node`/`nodejs`、`system`。 |
| `dependency_profile` | `stdlib` | 受服务器授权控制；普通 Skill 应省略。包内声明不会获得权限。 |
| `model_calls` | `0` | 脚本内模型调用次数，范围 0–3；确定性脚本应省略。 |
| `timeout_ms` | `timeout_seconds × 1000`，通常 30000 | 1–300000 毫秒。 |
| `allowed_artifact_paths` | `["artifacts"]` | 只能是 `artifacts` 或其子路径。 |
| `max_artifact_count` | `10` | 普通 Skill 最多 10 个；服务器授权可提高到 20 个。 |
| `max_artifact_bytes` | `2097152` | 普通 Skill 总计最多 2 MiB；服务器授权可提高到 256 MiB。 |
| `result_mode` | `mixed` | `stdout_json`、`stdout_text`、`artifacts` 或 `mixed`。 |
| `input_files` | 空 | 输入文件角色清单；最多 16 个角色，单次调用总计最多接收 16 个文件。 |

### 本地包声明

```json
{
  "bundled_packages": [
    {"name": "your_local_package", "path": "scripts/your_local_package"}
  ]
}
```

- 声明是可选的；未声明时仍按入口目录的标准导入规则自动识别本地模块。
- `name` 和 `path` 必须唯一，路径必须存在且不能包含符号链接或路径逃逸。
- Python 包必须能从入口文件所在目录直接导入。
- Node.js 本地包必须使用 `./` 或 `../` 相对导入；声明不会创建裸包别名。
- 本地包不会进入依赖指纹，也不会触发 Profile 构建。

## 输入文件

```json
{
  "input_files": [
    {
      "name": "source",
      "argument": "source_file_id",
      "required": false,
      "multiple": false,
      "extensions": [".txt", ".md"],
      "mime_types": ["text/plain", "text/markdown"],
      "max_bytes": 1048576
    },
    {
      "name": "attachments",
      "argument": "attachment_file_ids",
      "required": false,
      "multiple": true,
      "max_count": 3,
      "extensions": [".json"],
      "mime_types": ["application/json"],
      "max_bytes": 1048576
    }
  ]
}
```

| 字段 | 默认值 | 作用与限制 |
| --- | --- | --- |
| `name` | 无，必填 | `input_files.<name>` 的键，同时用于 Sandbox 目录；各项之间必须唯一。 |
| `argument` | 无，必填 | Tool 参数中承载 `file_id` 或 `file_id[]` 的字段；各项之间必须唯一。 |
| `required` | `false` | 缺失时是否在执行前失败。 |
| `multiple` | `false` | `false` 表示单文件，`true` 表示文件数组。 |
| `max_count` | 多文件时 `16` | 多文件最大数量，不能超过平台单次调用总上限 16。 |
| `extensions` | 空，不按扩展名过滤 | 允许的小写扩展名白名单；可写 `.txt` 或 `txt`，运行时归一化为 `.txt`。 |
| `mime_types` | 空，不按 MIME 过滤 | 允许的 MIME 白名单；参数会转为小写并去除 `;` 后的参数。 |
| `max_bytes` | 普通 Skill 为 10 MiB | 每个文件上限；服务器授权 Profile 可提高，但包内声明不能自行提权。 |

ZGI 下载已授权文件并写入 `inputs/`，随后在 stdin JSON 中增加文件元数据。脚本
只能读取注入的相对路径，不得自行下载 ZGI 文件。完整协议见
[输入文件协议](script-skill-input-files.md)。

## 输出模式和模型可见结果

当前没有 `script_output_schema`。输出契约由 `result_mode`、stdout 内容、退出码和
Artifact 声明共同组成；需要稳定结构时，stdout 顶层必须返回一个 JSON object，
不要直接返回数组、标量或 `null`，应把它们放在对象字段中。

| 模式 | stdout | 文件产物 | 推荐用途 |
| --- | --- | --- | --- |
| `stdout_json` | 一个 JSON object | 可不生成 | 结构化计算结果 |
| `stdout_text` | UTF-8 文本 | 可不生成 | 人类可读摘要 |
| `artifacts` | 可留空 | 至少一个文件 | 文件是唯一交付物 |
| `mixed` | JSON object 或文本 | 一个或多个文件 | 同时返回摘要和附件 |

模型实际收到：

1. 合法 JSON object stdout 转为 JSON Tool 消息；其他 stdout 转为 text 消息。
2. stderr 转为 log 消息，只能包含安全诊断信息。
3. 每个持久化产物转为 file Tool 消息。
4. 额外的 `artifacts` JSON 消息包含路径、大小、持久化状态、文件 ID 和下载地址。

Sandbox 路径不是下载地址。文件必须写入声明的 `artifacts/` 路径并在进程退出前
关闭；ZGI 会在删除临时 Sandbox 前持久化文件。完整链路见
[输出文件协议](script-skill-output-files.md)。

成功使用退出码 0；失败时向 stderr 写安全错误并返回非零退出码。不要把失败
伪装为成功 JSON。失败前产生的诊断产物可能仍被收集，但不代表调用成功。

## 依赖规则

依赖分为三类：

1. 本地代码：随 Skill 放入 `scripts/`，可通过 `bundled_packages` 声明。
2. Python/Node 第三方包：使用精确版本的 `requirements.txt`、`package.json` 或
   manifest dependencies，由部署侧构建不可变 Profile。
3. 系统工具：通过 `dependencies.system` 请求并由部署者审核、预装。

禁止未固定版本、远程 URL、本地路径依赖以及运行时安装命令。设置
`CODE_EXECUTION_SKILL_DEPENDENCY_SCAN_ENABLED=false` 后，声明不会触发扫描或构建；
默认 Profile 的 Python Skill 使用部署配置的预构建数据 Profile，该 Profile 必须已经
包含全部第三方包。默认 Profile 的 Node.js Skill 仍使用 `stdlib`；需要第三方包时必须
由部署管理员通过服务器控制的 runtime grant 绑定兼容 Node.js 的预构建 Profile，关闭
扫描本身不会安装 Node.js 包。

## 本地验证

Python 示例：

```bash
cd docs/examples/zgi-script-skill-template
python3 -m compileall scripts
python3 scripts/run.py < examples/request.json
```

Node.js 示例：

```bash
cd docs/examples/zgi-node-script-skill-template
node --check scripts/run.js
node --check scripts/zgi_skill_node_template/index.js
node scripts/run.js < examples/request.json
```

验证 stdout 是一个 JSON object、退出码为 0、只生成声明的
`artifacts/result.json`。还要使用缺失字段或错误类型验证非零退出码，并在目标
Profile 中执行一次真实 Sandbox 冒烟测试。

## 发布检查清单

- [ ] 名称、description 和适用范围一致。
- [ ] 输入 Schema 是封闭对象，并覆盖所有必需参数。
- [ ] manifest 明确声明入口、语言、超时、输出模式和产物限制。
- [ ] 本地包可按标准语言规则导入；可选声明与真实路径一致。
- [ ] 第三方依赖精确固定，并已进入目标 Profile。
- [ ] 单文件、多文件参数与 `input_files` 声明一致。
- [ ] stdout、stderr、退出码和 Artifact 行为符合契约。
- [ ] 不包含运行时安装、任意网络、后台任务、密钥或生成数据。
- [ ] Python/Node 本地冒烟和真实 Sandbox 冒烟通过。
