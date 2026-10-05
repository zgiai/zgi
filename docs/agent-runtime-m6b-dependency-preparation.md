# M6B 依赖准备 MVP

M6B 依赖准备支持两种明确来源：默认的“隔离用户空间、离线来源”，以及仅在 M6A `controlled-http` 白名单 Profile 上启用的在线来源。在线来源仍然是用户空间安装，不等同于任意联网安装或系统包安装。

## 开启方式

运行时 Profile 必须由运维在 `ZGI_AGENT_RUNTIME_PROFILES_JSON` 或 `ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON` 中显式标记：

```json
{
  "test": {
    "image": "registry.example/zgi-agent-runtime@sha256:<digest>",
    "dependency_preparation": true,
    "dependency_preparation_mode": "offline-bundled"
  }
}
```

`dependency_preparation_mode` 可为 `offline-bundled` 或 `controlled-http`，省略时由 Runtime Agent 向后兼容地补齐前者。`controlled-http` 只允许配合 `network_policy=controlled-http`、本地 gVisor Worker 和非空 `egress_allowed_hosts` 使用；E2B 当前仍拒绝该模式。其他值会在 Worker 侧拒绝。

同时，API 的 `ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON` 中对应 Profile 必须包含：

```json
{
  "test": {
    "backend": "local-gvisor",
    "organization_ids": ["<test-organization>"],
    "policy_ids": ["controlled-http"],
    "dependency_preparation": true,
    "dependency_preparation_mode": "controlled-http"
  }
}
```

缺少任一开关时，控制台 Agent 不会收到 `workspace_prepare_dependencies` 工具，已有 `shell_exec`、`run_script` 和旧 Sandbox 行为不变。

## Skill 包约定

工具要求当前 Skill 已通过 `load_skill` 激活，并使用当前 `session_scripts` 中的 `package_sha256`。支持的依赖来源是 Skill 包内的：

- Python：离线模式使用 `requirements.txt` 加 `vendor/python/*.whl`；受控在线模式使用相同的声明，通过 HTTP(S) 代理安装 wheel，不执行源码构建。
- Node.js：离线模式使用 `package.json` 的 `dependencies`/`optionalDependencies` 加 `vendor/node_modules/`；受控在线模式在会话 Home 生成临时清单并执行 `npm install --ignore-scripts`，不执行 npm 生命周期脚本。

没有依赖声明且脚本只使用标准库/包内本地模块时会返回 `no_dependencies`。如果脚本包含可识别的第三方 Python/Node import 但没有 `requirements.txt` 或 `package.json`，会返回 `dependency_manifest_required` 并列出检测到的包名；这不是联网重试，也不会在 API 宿主机安装包。声明依赖但没有离线来源时返回 `offline_dependency_source_missing`；受控在线模式的网络失败会返回带命令和 stderr 摘要的准备失败。两种模式都拒绝 `pyproject.toml`、远程 URL、Git、`file:`、系统包和任意安装命令。

准备结果写入会话 Home 的内容寻址目录，并以 Skill 摘要、依赖清单和来源模式计算的指纹复用。结果协议包含 `preparation_version`、`source_mode`、`fingerprint` 和 `cache_status`：离线仍为 v2，受控在线为 v3；首次成功准备为 `cache_status=created`，完整且仍存在的结果再次调用为 `status=reused`、`cache_status=reused`。复用前会校验 `prepare.json`、指纹以及 Python/Node 产物路径；缓存目录损坏或产物缺失时会自动重建，而不会把损坏结果报告为已复用。环境暂停、重建或重新连接后，只要 Home/Workspace 恢复，后续调用会返回 `reused`，不会重复安装。指纹改变或来源模式改变会生成新的目录，不覆盖旧结果。

当前准备协议版本为离线 `v2`、受控在线 `v3`。升级协议版本会使旧 Home 结果重新准备，避免旧格式的路径或安全假设被静默沿用。

模型收到相对工作区路径（例如 `.zgi-runtime-home/.zgi-dependencies/<skill>/<digest>/python/bin/python`）和 stdout/stderr 诊断。运行 Skill 时应显式使用返回的 Python 路径，或设置对应的 `NODE_PATH`；不要假设宿主机路径或系统全局包已改变。

## 安全边界与后续

依赖 Job 仍在已批准环境中执行，并受现有 Job 超时、输出、文件和资源限制约束。离线模式的网络必须为 `disabled`；受控在线模式只获得 loopback HTTPS 代理，Worker 对每次 CONNECT 执行域名、端口、DNS 和特殊地址检查，`*.` 规则只匹配一层子域名。安装结果不进入 API 控制面，也不授予系统 root、原始 TCP 或任意网络权限；Home 依赖缓存的每一级路径也必须是普通目录，符号链接缓存会被拒绝且不会跟随或删除其目标。

这一轮的退出条件是：无依赖、离线来源缺失、受控在线成功/失败、重复复用和环境重建均有可观察结果。任意用户空间联网、系统依赖构建、E2B 在线安装和共享缓存服务仍属于后续工作。
