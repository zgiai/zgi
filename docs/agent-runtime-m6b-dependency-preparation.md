# M6B 依赖准备 MVP

M6B 第一轮只开放“隔离的用户空间、离线来源”依赖准备。它用于验证依赖准备的生命周期、复用和失败语义，不等同于任意联网安装。

## 开启方式

运行时 Profile 必须由运维在 `ZGI_AGENT_RUNTIME_PROFILES_JSON` 或 `ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON` 中显式标记：

```json
{
  "test": {
    "image": "registry.example/zgi-agent-runtime@sha256:<digest>",
    "dependency_preparation": true
  }
}
```

同时，API 的 `ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON` 中对应 Profile 必须包含：

```json
{
  "test": {
    "backend": "local-gvisor",
    "organization_ids": ["<test-organization>"],
    "policy_ids": ["offline"],
    "dependency_preparation": true
  }
}
```

缺少任一开关时，控制台 Agent 不会收到 `workspace_prepare_dependencies` 工具，已有 `shell_exec`、`run_script` 和旧 Sandbox 行为不变。

## Skill 包约定

工具要求当前 Skill 已通过 `load_skill` 激活，并使用当前 `session_scripts` 中的 `package_sha256`。支持的依赖来源是 Skill 包内的：

- Python：`requirements.txt` 加 `vendor/python/*.whl`。只使用 wheel、`--no-index`，不执行源码构建。
- Node.js：`package.json` 的 `dependencies`/`optionalDependencies` 加 `vendor/node_modules/`。复制到会话 Home，不执行 npm 生命周期脚本。

没有依赖声明时会返回 `no_dependencies`。声明依赖但没有离线来源时返回 `offline_dependency_source_missing`；这不是联网重试，也不会在 API 宿主机安装包。`pyproject.toml`、远程 URL、Git、`file:`、系统包和任意安装命令目前不支持。

准备结果写入会话 Home 的内容寻址目录，并以 manifest 指纹复用。环境暂停、重建或重新连接后，只要 Home/Workspace 恢复，后续调用会返回 `reused`，不会重复安装。指纹改变会生成新的目录，不覆盖旧结果。

模型收到相对工作区路径（例如 `.zgi-runtime-home/.zgi-dependencies/<skill>/<digest>/python/bin/python`）和 stdout/stderr 诊断。运行 Skill 时应显式使用返回的 Python 路径，或设置对应的 `NODE_PATH`；不要假设宿主机路径或系统全局包已改变。

## 安全边界与后续

依赖 Job 仍在 gVisor/E2B 已批准环境中执行，网络必须为 `disabled`，并受现有 Job 超时、输出、文件和资源限制约束。安装结果不进入 API 控制面，也不授予系统 root 或任意网络权限。

这一轮的退出条件是：无依赖、离线来源缺失、成功准备、重复复用和环境重建均有可观察结果。带受控 HTTP(S) 的来源安装、任意用户空间联网、系统依赖构建和缓存服务属于后续 M6B/M6C，不在本 MVP 中宣称支持。
