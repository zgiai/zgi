# M6B 依赖准备与标准 Skill 按需安装

M6B 同时保留旧的声明式依赖准备，并增加可移植 Agent Skill 的按需安装模式。默认 `cc-codex-standard` Profile 预装常见语言、办公、PDF、媒体和原生编译工具；缺少包时，模型可在同一隔离会话中使用 pip/npm，或调用受控 apt facade。网络只允许批准包源的 HTTPS，不提供任意外网。这个边界针对目的地，不区分调用它的是包管理器还是其他 HTTP 客户端。

标准 Skill 不要求 ZGI manifest、`requirements.txt` 或 `package.json`。声明式依赖工具和旧 `run_script` 继续工作，但不再是标准 Skill 的默认路径。

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

`dependency_preparation_mode` 可为 `offline-bundled` 或 `controlled-http`，省略时由 Runtime Agent 向后兼容地补齐前者。`controlled-http` 只允许配合 `network_policy=controlled-http`、本地 gVisor Worker 和非空 `egress_allowed_hosts` 使用；E2B 当前仍拒绝该通用模式。E2B 的标准 `package-install` 是另一条受控路径：它使用 E2B 的 `allowOut` 包源白名单和 `network_mode=package-install`，不提供任意 HTTPS。其他值会在 Worker 侧拒绝。

标准 Profile 的 Worker 配置由运维注入，示例：

```json
{
  "cc-codex-standard": {
    "image": "registry.example/zgi/agent-runtime@sha256:<digest>",
    "dependency_installation": true,
    "package_network_mode": "package-install",
    "egress_allowed_hosts": ["pypi.org", "files.pythonhosted.org", "registry.npmjs.org", "deb.debian.org", "security.debian.org"]
  }
}
```

API 的批准 Profile 可以标记 `dependency_installation=true` 和 `package_network_mode=package-install`。这两个字段是服务端配置，不进入 Agent 编辑器；解析后 API 会强制绑定使用 `package-install`，不会接受用户提交的网络策略。

对应的 API 侧批准项需要把 `package-install` 放入内部允许集合（例如 `policy_ids:["offline","package-install"]`）；普通用户保存的 Agent 配置仍只有 `execution_runtime.enabled`。

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

缺少依赖准备开关时，控制台 Agent 不会收到 `workspace_prepare_dependencies` 工具，已有 `shell_exec`、`run_script` 和旧 Sandbox 行为不变。启用 `dependency_installation` 的标准 Profile 只通过 Shell 使用 pip/npm/apt，即使同时保留旧准备开关也不暴露声明式工具；旧声明式 Profile 继续使用旧工具。

`shell_exec` 的说明展示服务端批准的网络模式，工具结果的 `runtime_capabilities` 返回当前环境实际的 `network_mode`、`dependency_installation` 和 `package_network_mode`，不返回内部代理地址或凭据。已有会话可能保留配置变更前的环境，模型应以工具结果的实际能力为准，不能把所有环境描述为离线或假定具备任意联网权限。

E2B 的 Runtime Agent 和包管理器能力必须烘焙进模板，而不能只依赖创建
Sandbox 时传入的 `envVars`。使用仓库的 `sandbox/Dockerfile.agent-runtime-e2b`
重新构建并替换批准的模板；Worker 会在环境就绪和恢复时校验 Runtime Agent
的能力。离线 Profile 必须声明 `offline-bundled`；标准包安装 Profile 必须
同时声明 `package-install`，并报告 `package_network_enforced=true`，否则会
fail-closed，不会把依赖错误地当成可安装。

## Skill 包约定

工具要求当前 Skill 已通过 `load_skill` 激活，并使用当前 `session_scripts` 中的 `package_sha256`。支持的依赖来源是 Skill 包内的：

- Python：离线模式使用 `requirements.txt` 加 `vendor/python/*.whl`；受控在线模式使用相同的声明，通过 HTTP(S) 代理安装 wheel，不执行源码构建。
- Node.js：离线模式使用 `package.json` 的 `dependencies`/`optionalDependencies` 加 `vendor/node_modules/`；受控在线模式在会话 Home 生成临时清单并执行 `npm install --ignore-scripts`，不执行 npm 生命周期脚本。

旧声明式路径仍会返回 `no_dependencies`、`dependency_manifest_required`、`offline_dependency_source_missing` 等诊断。标准路径不因静态扫描结果阻止导入：模型先运行脚本，根据 `ModuleNotFoundError`、`Cannot find module` 或 `command not found` 判断缺失项，再在包安装模式中执行 `pip install`、`npm install` 或 `apt-get install -y`。pip/npm 安装写入会话 Home/Workspace；apt 请求由 Runtime Agent 的 root 进程校验后执行，Job 始终以 UID 65532 运行。APT 包名、操作和选项有独立校验；普通 pip/npm 目前依赖网络目的地限制及文件权限，不应宣称其所有 Git、URL、file 参数均经过统一包装器拒绝。本地文件依赖只能使用沙箱中当前 Job 可读取的文件。

准备结果写入会话 Home 的内容寻址目录，并以 Skill 摘要、依赖清单和来源模式计算的指纹复用。结果协议包含 `preparation_version`、`source_mode`、`fingerprint` 和 `cache_status`：离线为 v4，受控在线为 v5（v4/v5 发布的 Python 环境中，console_scripts 和激活脚本指向最终路径；更早版本的结果会重建而不复用）；首次成功准备为 `cache_status=created`，完整且仍存在的结果再次调用为 `status=reused`、`cache_status=reused`。复用前会校验 `prepare.json`、指纹以及 Python/Node 产物路径；缓存目录损坏或产物缺失时会自动重建，而不会把损坏结果报告为已复用。环境暂停、重建或重新连接后，只要 Home/Workspace 恢复，后续调用会返回 `reused`，不会重复安装。指纹改变或来源模式改变会生成新的目录，不覆盖旧结果。

当前准备协议版本为离线 `v2`、受控在线为 `v3`。标准按需安装中，Python/Node 安装目录和缓存随 Workspace/Home 检查点恢复；系统包 ledger 当前只记录经校验的 APT 参数和安装时间，计算重建后自动重放。恢复失败会阻止用户 Job 启动，后续请求可重试，不会把一次失败标记为永久恢复成功。它尚不是覆盖三个包管理器的完整版本账本：Profile digest 指纹、实际解析版本、跨会话依赖集缓存和 rootfs 增量层仍需后续实现。APT 重放可能获取当前包源版本，不能宣传为版本锁定恢复。

标准 Profile 的运行时会把 `HOME/.venv/bin` 放在 PATH 首位，并设置 `VIRTUAL_ENV`；`shell_exec` 使用非登录 Shell，避免 `/etc/profile` 覆盖注入的 PATH/HOME。因此 `pip install` 与 `python -m pip install` 会写入当前会话的独立虚拟环境。Profile 同时预装 `python3-venv`，确保 Debian 基线和最小化测试镜像都能创建该虚拟环境。Node 包写入当前工作区的 `node_modules`，npm cache 留在 Home。基础语言与 Profile 目录归 root 所有且 Job 不可写，默认前缀的 npm 全局安装不能修改它们。旧声明式准备模型收到相对工作区路径（例如 `.zgi-runtime-home/.zgi-dependencies/<skill>/<digest>/python/bin/python`）和 stdout/stderr 诊断；不要假设宿主机路径或系统全局包已改变。

标准镜像同时保留与 Node 二进制匹配的 `/usr/local/include/node` 头文件；Runtime Agent 注入 `NPM_CONFIG_NODEDIR=/usr/local`，使 node-gyp 原生构建无需访问额外的 Node 下载站。自定义运行时镜像应保留这一布局及编译工具，不能以开放更多外网来弥补头文件缺失。

每个 Linux Job 的 `TMPDIR`、`TMP`、`TEMP` 指向工作区内独立的 `.zgi-runtime-tmp/job-<随机标识>`，而不是本地后端容量较小的共享 `/tmp`。这属于可丢弃的构建数据，不是依赖缓存或成果目录；脚本不能将需要保留的结果放在这些路径。成功、非零退出、取消和超时都在确认进程组已停止后回收该目录，启动失败也会回收。清理失败或仍有后台进程时环境拒绝继续执行，不将未完成的清理报告为成功。重建 Runtime Agent 时仅回收保留工作区内具有受控名称的旧 Job 临时目录；这一操作要求 Worker 独占该工作区且旧计算已停止。检查点不包含 `.zgi-runtime-tmp`，导入也拒绝该保留路径。pip/npm 的有效缓存、虚拟环境、依赖归档及业务文件不受影响。

临时数据使用工作区存储，仍占用部署的磁盘容量；本机制不是新的磁盘配额服务。显式绕过 `TMPDIR` 写入 `/tmp` 的程序仍受后端 `/tmp` 容量限制，任意系统安装的中断也不承诺事务回滚。

root Runtime Agent 上传和恢复的文件、父目录会通过工作区内的文件描述符交给 Job UID/GID，避免非 root 脚本无法读取输入或解包 Skill。检查点支持包管理器生成的工作区内相对符号链接；解包时先写普通文件、最后恢复链接并校验完整链接链，拒绝绝对、越界、循环、悬空链接和设备文件。Skill ZIP 的导入限制不变，仍不接受符号链接。

## 安全边界与后续

标准 Profile 预装 `file`，并在只读的 `/opt/zgi-profile/resources/xml/` 中提供 Dublin Core / W3C Schema、递归 DTD、XML Catalog、来源与许可证及 SHA-256 清单。`XML_CATALOG_FILES` 让 libxml2/lxml 将已收录的上游 URL 解析到离线资源，保留原始 DOCX Skill 的 XSD 校验；这不开放非包源网络，也不替任意 HTTP 下载提供代理。Profile 构建检查递归导入、有效和无效 XML 及未知 URL 拒绝。更新资源需要重新构建本地镜像及 E2B 模板，两种环境使用同一份资源。

依赖 Job 仍在已批准环境中执行，并受现有 Job 超时、输出、文件和资源限制约束。离线模式的网络必须为 `disabled`；本地受控在线模式只获得 loopback HTTPS 代理，Worker 对每次 CONNECT 执行域名、端口、DNS 和特殊地址检查，`*.` 规则只匹配一层子域名。E2B 标准包安装模式同时使用供应商 deny/allowOut 和模板内的 root HTTPS 代理；nftables 只允许 Job 访问 `127.0.0.1:3128`，其他 TCP/UDP/IPv6 和 link-local 访问被拒绝，root/envd 保留启动所需通信。规则或权限准备失败时 Runtime Agent 不启动，不能降级为任意联网。模板可用 `ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON` 配置包源；它应与 Worker 批准名单一致，缺省仅为 PyPI、npm、Debian 的五个公共域名。该配置只由运维注入，Job 不继承运行时控制变量。

Runtime Agent 负责受控 APT 安装，子 Job 仍为 UID 65532，并通过 `setpriv --no-new-privs` 防止执行 setuid 或文件 capability 程序获取新权限。安装结果不进入 API 控制面，也不授予系统 root 或任意网络权限；Home 依赖缓存的每一级路径也必须是普通目录，符号链接缓存会被拒绝且不会跟随或删除其目标。

APT 请求在单个环境中串行执行，排队可取消；断开请求、关闭 Runtime HTTP Server 或 Backend 会取消活动安装并清理其进程树。`ZGI_RUNTIME_AGENT_PACKAGE_TIMEOUT_MS`（默认 600000）和 `ZGI_RUNTIME_AGENT_PACKAGE_OUTPUT_BYTES`（默认每个输出流 262144 字节）仅由运维设置，Job 不继承这些控制变量。输出边读边截断而不是运行结束后才限额。系统包账本限定在工作区，拒绝链接与非普通文件，读取上限 1 MiB / 1000 条命令；更新采用唯一临时文件和原子替换，重复成功命令不重复追加。安装取消不保证回滚已执行的 dpkg 步骤，后续恢复仍需显式诊断，不把取消报告为事务回滚。

这一轮的退出条件是：标准 Skill 无 manifest 可导入；常见包来自预构建 Profile；pip/npm/apt 安装与失败诊断可观察；重复执行和环境重建不丢失安装状态；非包源 HTTPS、原始 TCP、私网和代理绕过被拒绝。真实 E2B 和 gVisor 验收必须同时验证原生编译、非 root/无新增权限、暂停恢复、检查点销毁重建和失败恢复，不能由 Mock 替代。

原样 Skill 中如果包含从非包源下载验证 schema、模型、浏览器或其他资源的步骤，安装代码包不能自动补齐这些资源。需要作者随包提供资源，或由运维纳入审核的预构建 Profile；不能以开放任意外网来掩盖此类失败。
