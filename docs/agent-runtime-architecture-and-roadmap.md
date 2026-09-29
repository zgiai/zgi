# ZGI Agent 执行环境架构设计与渐进改造计划

## 1. 文档信息与总体结论

- 文档状态：架构设计与实施计划，不表示相关功能已经实现或测试通过。
- 代码基线：`d039faee8`；进入实施阶段前重新核对远端 `main` 的实际差异。
- 调研日期：2026-09-26。
- 面向读者：后端、平台、前端、测试、运维及 Skill 开发者。

### 总体结论

ZGI 应借鉴 Dify 的“环境资源模型 + 统一执行接口 + 多后端适配”机制，而不是直接搬入 Dify 的 Agent 引擎或整个部署栈。

最终形成三种执行后端：

1. **本地后端**：Linux 主机上的 containerd + gVisor，适合本地部署和低成本运行。
2. **托管云后端**：E2B 虚拟机环境，先交付线上虚拟机能力。
3. **自建虚拟机后端**：containerd + Kata Containers + KVM，后续交付自主可控的线上虚拟机能力。

三个后端共用 Agent 工具、Skill 运行逻辑、文件协议、Job 协议和授权模型。差异封装在后端适配器中，不能渗透到模型提示词或 Skill 开发接口。

本地 gVisor **不是完整硬件虚拟机**；E2B 和 Kata 提供虚拟机隔离。统一的是使用契约，而不是宣称三者具有相同隔离强度。

### 已确定的产品边界

- 本地和 E2B 优先完成，自建 Kata 分期交付。
- Agent 显式开启执行环境，默认关闭。
- 环境按会话绑定，不同用户、组织和会话不共享可写目录。
- 空闲释放计算资源，会话文件默认保留七天。
- Home Snapshot 作为 Agent 可复用的环境模板。
- 从离线执行逐步扩展到用户空间安装、受控 HTTP(S)、系统依赖构建和平台 CLI。
- 本轮先支持认证后的控制台 Agent；不同时开放匿名 WebApp、公开 API 和外部终端。
- 不开放任意 SSH、数据库 TCP、宿主机 Docker socket、root Shell 或任意网络访问。

## 2. 代码现状与改造依据

### 2.1 Dify 中值得借鉴的机制

Dify 当前将环境区分为 Home Snapshot、Workspace、Execution Binding 和临时 RuntimeLease。后端负责物理资源，调用过程只持有临时连接；本地与 E2B 使用统一的命令和文件接口。参考 [Dify 运行时契约](https://github.com/langgenius/dify/blob/f4602cc1fe8448486e185be74152699322ccec3f/dify-agent/src/dify_agent/runtime_backend/protocols.py)。

Dify 本地环境采用共享容器与路径隔离，E2B 后端则把 Binding 和 Workspace 物理生命周期耦合在同一个云环境中，因此并不是所有后端都支持共享 Workspace 或保留 Workspace 后单独销毁 Binding。参考 [Dify 资源模型](https://github.com/langgenius/dify/blob/f4602cc1fe8448486e185be74152699322ccec3f/dify-agent/docs/dify-agent/concepts/runtime-resources/index.md)、[E2B 适配实现](https://github.com/langgenius/dify/blob/f4602cc1fe8448486e185be74152699322ccec3f/dify-agent/src/dify_agent/runtime_backend/e2b.py)。

ZGI 应吸收资源拆分和后端抽象，但保留“每个 Binding 独立隔离环境”的本地方案，不采用共享容器作为生产默认值。

### 2.2 ZGI 已有能力和实际缺口

| 子系统 | 现有实现 | 本次处理 |
|---|---|---|
| Agent 执行 | 已有 Skill Loop、RuntimeTool、持续运行上下文、运行租约和停止机制 | 直接接入执行环境，不重建 Agent 引擎 |
| Script Skill | 创建 Sandbox、上传文件、同步执行、下载 Artifact、删除 Sandbox | 保留默认行为，新增会话环境执行路径 |
| Sandbox 资源 | `Sandbox` 同时承载目录、TTL、依赖 Profile、归属和 Worker 信息 | 新增环境资源模型，不继续向单个结构堆叠职责 |
| 执行器 | 同步执行，进程级并发控制 | 增加异步 Job、PTY、游标日志、输入和取消 |
| 生命周期 | 到期主要修改状态；删除忽略部分文件清理错误 | 增加独立回收器、重试和资源对账 |
| 文件访问 | 大量操作直接依赖本机 `RootPath` | 抽象统一数据面，远端不能回落到宿主机文件操作 |
| 依赖构建 | 已有依赖扫描、Profile、构建队列和校验 | 保留旧链路；新增独立环境准备与隔离构建 |
| Worker | 已有 WorkerID、地址和归属代理 | 补齐注册、心跳、容量调度和故障处理 |
| 前端 | 已有 Agent 配置、版本发布和工具事件展示 | 增加环境配置、状态和文件入口 |
| 部署 | PostgreSQL、Redis、Sandbox 服务和本地卷 | 分阶段增加 Worker、镜像、对象存储和网络网关 |

关键代码依据：

- [生命周期管理](../sandbox/internal/lifecycle/manager.go)：`expireIfNeeded` 目前没有完成物理资源回收，删除路径也需要可靠的失败处理。
- [同步执行接口](../sandbox/internal/runner/runner.go)：当前 `CommandSpec` 和同步执行接口不能表达长期 Job 和交互输入。
- [Agent 工具契约](../api/internal/capabilities/chatruntime/skillloop/types.go)：可复用 RuntimeTool，但需区分模型结果、trace 和会话 transcript 的保存规则。
- [Agent 运行租约](../api/internal/capabilities/chatruntime/service/runtime_execution.go)：已有持续执行、运行所有权和续约机制，应复用而不是另起一套运行循环。
- [Agent transcript](../api/internal/capabilities/chatruntime/service/agent_transcript.go)：模型协议消息可能进入会话历史，不能只审查 trace。
- [Sandbox 执行器](../sandbox/internal/executor/service.go)：脚本、文件与本机目录紧密耦合，远端后端接入前需要抽象数据面。
- [Sandbox 配置](../sandbox/internal/config/config.go)：网络能力不能继续仅根据后端名称推断。

另外：

- 顶层 `runner/` 是插件运行服务，不改造为 Agent Runtime。
- `runtimeworkspace` 是对外访问与授权相关模块，不用它命名新的文件执行环境。
- API 新增执行环境领域模块使用 `agentexecution`；Sandbox 内部使用 `agentruntime`。
- 后端能力必须通过真实探测确认，不能仅根据配置名称宣称安全保证。

## 3. 目标架构与资源模型

### 3.1 服务划分

```text
Web 控制台
    │
    ▼
ZGI API
  Agent 配置 / Skill Loop / 权限与审批 / 文件与模型能力
    │ 可信调用：组织、用户、Agent、会话、运行 ID
    ▼
Sandbox Control Plane
  Binding / Workspace / Snapshot / Job 元数据
  调度 / 配额 / 生命周期 / 后端适配器
    ├── Local Worker ── containerd + gVisor
    ├── E2B Adapter ── E2B 云虚拟机
    └── Kata Worker ── containerd + Kata + KVM
                             │
                      环境内 Runtime Agent
                      命令 / PTY / 文件 / 日志

配套：
  PostgreSQL：持久状态与任务队列
  Redis：缓存和通知，不承担唯一状态源
  对象存储：文件检查点、Home Snapshot、构建产物
  OCI Registry：本地/Kata 基础镜像
  Egress Gateway：受控联网
```

#### ZGI API

- 管理 Agent 环境配置及发布版本。
- 校验当前用户和运行入口是否有权启用环境。
- 向模型注入执行工具。
- 调用现有文件、模型、连接器和审批服务。
- 不直接访问 containerd、KVM 或云供应商凭据。

#### Sandbox Control Plane

- 在现有 `sandbox/` Go 模块中增量实现。
- 管理逻辑资源、配额、调度、状态机和后端适配。
- 对外仅提供受认证的内部 API。
- 不执行用户命令，不持有宿主机容器管理 socket。

#### Runtime Worker

- 从同一 Go 模块构建独立 Worker 二进制。
- 本地/Kata Worker 管理节点上的容器和持久卷。
- 持有节点级特权，必须与 API、控制面和用户进程隔离。
- E2B 由控制面适配器调用供应商 API，不要求部署本地 KVM Worker。

#### 环境内 Runtime Agent

- 管理用户进程、PTY、输出、文件和 Job 状态。
- 管理进程与用户进程使用不同权限。
- 管理凭据、Job 元数据和控制 socket 不放入用户可读目录。
- 本地/Kata 通过管理通道访问，E2B 使用经过认证的 HTTPS 数据面。

### 3.2 资源定义

| 资源 | 用途 | 生命周期 |
|---|---|---|
| RuntimeProfile | 后端、基础镜像、能力和资源规格 | 管理员配置 |
| HomeSnapshot | 不可变环境模板，包含用户空间依赖和工具 | 被 Agent 版本引用期间保留 |
| ExecutionWorkspace | 当前会话的输入、工作文件和输出 | 默认最后使用后七天 |
| ExecutionBinding | 会话与环境的逻辑绑定，固定 Profile、模板和策略版本 | 独立于计算进程存活 |
| RuntimeLease | 一次操作使用的连接、客户端和临时凭据 | 调用内存对象，不持久化 |
| AdmissionLease | 运行所有权、过期时间和 fencing generation | 数据库持久化 |
| ExecutionJob | 一次命令或脚本执行 | 持久状态、可查询和取消 |
| EnvironmentBuild | 环境准备或系统镜像构建任务 | 独立 Job，有明确产物和失败状态 |

数据库保存资源 ID、归属、状态、版本、后端不透明引用和检查点，不保存连接对象或供应商访问令牌。

### 3.3 归属与绑定

活动 Binding 的唯一归属：

```text
organization + authenticated subject + agent + conversation
```

- Workspace 对应执行目录，不等同于 ZGI 产品中的组织工作空间。
- 创建 Binding 时固定 Agent 版本、HomeSnapshot 和策略版本。
- 修改 Agent 配置只影响新 Binding。
- 现有会话通过明确的“重建环境”操作应用新模板，先保存文件，再生成新的 Binding generation。
- 不允许模型通过参数指定组织、用户、供应商端点或后端凭据。
- 不同后端之间不共享可写物理卷；迁移必须导出、校验和恢复。

### 3.4 后端能力矩阵

| 能力 | 本地 gVisor | E2B | 自建 Kata |
|---|---|---|---|
| 隔离 | 用户态内核沙箱 | 云虚拟机 | KVM 虚拟机 |
| 命令、Job、PTY、文件 | 统一接口 | 统一接口 | 统一接口 |
| 空闲释放计算 | 停止环境，保留卷 | 暂停或检查点后释放 | 停止 VM，保留卷 |
| 恢复保证 | 文件恢复 | 文件恢复；可选进程恢复 | 文件恢复 |
| HomeSnapshot | 文件快照 | 文件快照与供应商优化快照 | 文件快照 |
| 跨后端恢复 | 兼容环境内的文件恢复 | 同左 | 同左 |
| 受控网络 | 节点网络隔离＋网关 | 供应商网络限制＋网关 | 节点网络隔离＋网关 |
| 本地硬件要求 | Linux，不要求 KVM | 不要求本地 KVM | Linux、KVM、可用 `/dev/kvm` |

Kata 默认使用 runtime-rs + Dragonball；不同时支持多个 VMM，避免首期测试矩阵膨胀。官方当前将 runtime-rs 描述为 Kata 4 的生产组件，并提供内置 Dragonball。参考 [Kata runtime-rs](https://github.com/kata-containers/kata-containers/blob/main/src/runtime-rs/README.md)。

所有后端通过实际探测报告能力；不能仅根据配置名称宣称支持网络隔离、硬磁盘配额或进程恢复。不满足 Profile 必需能力时拒绝创建，禁止静默降级。

## 4. 接口、执行链路与 Skill 兼容

### 4.1 Agent 配置

新增可选配置：

```json
{
  "execution_runtime": {
    "enabled": false,
    "profile_id": "local-standard",
    "home_snapshot_id": null,
    "policy_id": "offline-standard"
  }
}
```

约定：

- 缺失字段等同关闭，旧 Agent 无需迁移。
- Profile 和 Policy 只能从当前组织允许的目录选择。
- 草稿、发布快照和配置读取保持一致。
- 发布时验证引用存在、归属正确且能力满足要求。
- 公共展示配置不返回环境内部引用、供应商端点或凭据。
- 非控制台入口在本阶段不得因为配置开启而自动获得执行工具。

### 4.2 后端接口

采用窄接口，分别表达：

- Binding 创建、恢复、释放计算和销毁。
- HomeSnapshot 创建、恢复和删除。
- Job 启动、等待、读取输出、输入和取消。
- 文件上传、下载、列举、删除及 Artifact 导出。
- 能力与健康探测。

统一语义：

- 创建必须支持请求幂等键。
- 恢复已丢失的 Binding 返回明确错误，不能暗中重建空环境。
- 暂时不可达与永久丢失分别处理。
- 不支持保留文件后销毁计算资源的操作必须提前拒绝，不能先删除。
- 后端适配器自行处理物理引用；业务层只使用逻辑 ID。

### 4.3 内部 API

新增 `/v1/agent-runtime/` 路由组，保留旧 `/v1/sandboxes` 和 `/v1/exec/*`。

新增资源接口覆盖：

| 资源 | 操作 |
|---|---|
| Profiles | 可用能力目录、健康状态 |
| Bindings | 创建、查询、恢复、暂停、重建、删除 |
| Jobs | 创建、查询、等待、游标输出、输入、取消 |
| Files | 流式上传下载、列举、Artifact 导出 |
| Snapshots | 创建、查询、删除 |
| Builds | 环境准备、构建状态和日志 |

- 组织、用户和运行归属来自 API 的可信调用上下文。
- 请求必须校验 Binding ownership 和 generation。
- 错误使用现有错误返回体系；API 增加错误码时同时补齐中英文。
- 明确区分权限不足、能力不支持、配额不足、环境丢失、暂时不可达、输入无效和命令失败。
- HTTP 超时不等于 Job 失败；调用者可继续用 Job ID 查询。

### 4.4 模型工具

新增第一方工具：

- `shell_run`
- `shell_wait`
- `shell_tail`
- `shell_input`
- `shell_terminate`
- `workspace_list`
- `workspace_read`
- `workspace_write`
- `workspace_export`

Shell 允许正常命令组合、管道和重定向。安全边界是隔离环境与能力授权，不是命令关键字黑名单。

工具返回：

- Job ID、状态、退出码。
- 有界 stdout/stderr、输出游标和截断标记。
- 可恢复错误分类。
- 已持久化文件的标准引用。

命令路径、Sandbox 路径和 E2B 地址均不能伪装成下载链接。文件必须通过现有文件服务生成下载卡片。

### 4.5 Job 语义

- 非 PTY 模式分别返回 stdout/stderr；PTY 模式返回合并终端流。
- `shell_run` 短暂等待后返回结果或运行中的 Job ID。
- `shell_wait` 单次最长等待三十秒，不无限占用 HTTP 连接。
- Job 在工具调用返回后继续执行，但默认不能超出所属 Agent Run 的生命周期。
- 用户停止或 Run 被替换时，终止旧 generation 的进程组和后代进程。
- 前端断开不等于停止 Agent，沿用现有持续执行机制。
- Agent 暂停等待审批时保存文件并终止未完成 Job；不承诺后台进程永久存活。
- 输出达到上限后继续排空管道，避免进程因日志阻塞。

### 4.6 两种 Skill 模式并存

#### 原有 Script 模式

- 保留输入 Schema、manifest、依赖 Profile、授权、输出模式和 Artifact 契约。
- 默认仍采用一次性 Sandbox。
- 扫描器开关不改变缺失依赖的安装语义。
- 不因为新环境上线而放宽旧运行权限。

#### 执行环境模式

- 可读取普通 `SKILL.md` 和脚本文件，不要求额外 manifest。
- 激活 Skill 后才复制对应版本包进入会话环境。
- 模型可以检查文件、运行辅助脚本、修复参数并组合 CLI。
- 原始导入包保持不变；环境中的副本可以产生缓存和临时文件。
- 显式依赖声明仍是准备环境的重要依据，未声明依赖也可通过环境准备流程安装。
- 静态扫描作为提示和预检，不充当执行环境的权限边界。

已有 `run_script` 增加显式的会话环境执行路径，仍由公共执行器处理参数、文件输入和输出解释，不在 API 重新实现脚本运行规则。

### 4.7 依赖自由度

分层解决，不能仅关闭扫描器：

1. **本地包**：复制后按 Python/Node 标准导入规则运行，不触发安装。
2. **用户空间依赖**：在 Home 内创建 venv、Node 项目环境或用户工具目录。
3. **缺失依赖**：返回环境准备需求；经策略允许后安装并记录实际结果。
4. **系统依赖**：进入独立构建任务，生成新基础环境版本。
5. **冲突依赖**：优先使用 Skill 独立 venv 或 Node 安装目录；无法兼容的系统依赖使用不同模板。

不能把现有两个 Profile 的 venv 简单拼接，就宣称系统 rootfs 已兼容。

新执行环境允许授权后的用户空间安装，不受旧命令安装拦截器影响；旧模式继续维持当前行为。

## 5. 持久化、安全、调度与部署

### 5.1 文件与快照

统一目录：

```text
/home/zgi       用户空间依赖、工具和 Skill 副本
/workspace      输入、工作文件和 artifacts
/tmp            有容量限制的临时目录
```

规则：

- 空闲三十分钟后释放计算资源。
- 会话 Home 和 Workspace 默认最后使用后保留七天。
- 七天指会话保留期，不是供应商持续运行时间。
- 被 Agent 发布版本引用的 HomeSnapshot 持续保留；无引用快照保留三十天后回收。
- 最低公共恢复能力是文件恢复，不承诺跨后端恢复内存、进程和连接。

持久化分两层：

- 供应商原生暂停或快照用于快速恢复。
- 标准化文件检查点用于七天保留、备份和受控迁移。

E2B 支持暂停及文件系统恢复，但具体套餐、版本和恢复方式必须在适配器验收中确认，不把它们当作永久不变的保证。参考 [E2B 官方 API 契约](https://github.com/e2b-dev/E2B/blob/main/spec/openapi.yml)。

快照：

- HomeSnapshot 不包含 Workspace 业务文件。
- 不包含管理凭据、CLI 临时令牌、socket、设备文件、FIFO 和运行时锁。
- 记录基础镜像、架构、Python/Node 版本及环境摘要。
- 跨后端复制仅在基础环境兼容时恢复；不承诺任意二进制环境可移植。
- 快照前停止写入 Job，生成文件清单和校验和，提交成功后再释放源计算资源。
- 检查点失败时保留源资源，并产生可重试任务和告警。

导入包继续禁止逃逸和符号链接；运行时 Home 使用独立的安全文件规则，允许 venv/npm 所需的内部符号链接，拒绝指向授权根目录之外的链接。不能全局放松现有校验。

### 5.2 网络

策略分三类：

- `offline-standard`：用户进程不能访问外网。
- `package-build`：准备环境时允许访问批准的软件仓库。
- `restricted-http`：运行时访问批准的 HTTP(S) 目标。

授权范围是：

```text
组织允许 ∩ Agent 配置 ∩ 当前运行授权 ∩ 后端可执行能力
```

要求：

- 网关规则由平台配置，Skill 文档和模型参数不能自行授权。
- 禁止宿主机、内网、云元数据地址、其他租户资源和 DNS 重绑定绕过。
- 本地/Kata 在网络层强制用户流量经过网关。
- E2B 使用供应商出站限制，只放行受控网关等必要地址。
- 仅设置 `HTTP_PROXY` 不算网络隔离。
- 同时验证原始 TCP、IPv6、代理绕过和 HTTP 重定向。
- 不支持所需限制的后端不得启用该网络策略。
- 平台 CLI 通道单独授权，不代表开放普通互联网。

### 5.3 CLI 与密钥

新增 `zgi-runtime` CLI，逐步提供：

- Skill 文件获取。
- 输入文件下载。
- Artifact 上传。
- 经治理的模型调用。
- 已绑定平台工具调用。

CLI 通过 Job/Run 级短期令牌访问 API Stub，不携带数据库密码、模型 API Key 或 E2B 管理密钥。

平台工具调用继续使用现有权限、审批和审计，不能借 CLI 绕过审批，也不创建另一套 Agent 调度循环。

令牌在 Run 结束、Binding generation 变化和权限撤销后立即失效。默认不直接把长期密钥注入用户进程；日志脱敏不能替代这个边界。

### 5.4 日志与会话历史

分别管理四类数据：

- **审计 trace**：ID、状态、资源消耗和经过清理的参数摘要。
- **模型 Tool 内容**：有界、脱敏后的输出。
- **会话 transcript**：按现有历史链路保存的 Tool 内容，增加大小与敏感内容控制。
- **原始 Job 日志**：受权限控制，默认七天保留，不自动全文进入模型上下文。

不能因为使用 `ModelResult` 就宣称输出完全不落库；必须测试 transcript 和异常路径。

### 5.5 调度与资源回收

新增 Worker 注册、心跳、容量和健康状态。

- PostgreSQL 是任务和租约的唯一持久状态源。
- 数据库锁与唯一约束负责抢占；generation 防止旧 Worker 写回新运行。
- Binding 默认单一执行所有者，拒绝并发写入型 Agent Run。
- 操作租约默认三十秒续约，九十秒失联后撤销。
- Worker 失联不能直接在另一节点重建空目录。
- 独立回收器对账数据库、容器、云实例、卷、快照和未完成构建。
- 删除流程先标记 `deleting`，清理完成才进入 `deleted`。
- 失败清理可重试，重复删除幂等，不能吞掉错误。

初始标准规格：

| 项目 | 默认值 |
|---|---|
| Binding 计算规格 | 1 vCPU、1 GiB 内存 |
| 单 Job 超时 | 300 秒；策略上限 1800 秒 |
| 单 Binding 活跃 Job | 2 |
| 单组织活跃 Binding | 2 |
| 单次模型输出 | 64 KiB |
| 原始输出 | stdout/stderr 各 1 MiB，超限标记截断 |
| Workspace 配额 | 1 GiB |
| Home 配额 | 2 GiB |

这些是新 Profile 默认值，不覆盖旧 Sandbox 配置。文件数、节点总容量和组织存储额度单独配置。

磁盘限制必须报告实际强度：本地/Kata 使用有界卷或文件系统配额；E2B 按供应商磁盘能力实施总量上限，分目录限制不足时明确标记，不能把执行后的目录扫描描述为硬限制。

### 5.6 部署形态

#### 本地部署

- API、Web、PostgreSQL、Redis、Sandbox Control Plane。
- Linux Worker、containerd、runsc 和持久卷。
- Worker 优先作为主机服务运行。
- Windows/macOS 开发者通过 Linux VM 或合适的 Linux 开发环境运行 Worker，不使用宿主机进程冒充安全后端。

#### E2B 部署

- 在本地部署基础上启用 E2B Adapter 和版本锁定的环境模板。
- 云密钥只放在控制面密钥配置中。
- 不需要在 ZGI 主机安装 KVM。
- 引入对象存储后提供可验证的文件保留闭环。

#### 自建 Kata 部署

- 独立 KVM 节点池、containerd、Kata、Dragonball、镜像仓库和持久卷。
- 节点必须通过硬件虚拟化和磁盘配额检查。
- 不假设现有服务器支持嵌套虚拟化。
- 首期不引入 Kubernetes、Nomad 或整套自建 E2B 平台。

## 6. 推进阶段与每一步 MVP

实施采用“纵向可运行切片”：每个阶段包含实现、测试、演示、文档和回滚，不先铺大量无使用者的抽象。

| 阶段 | 主要交付 | 可演示 MVP | 必须通过的验收 |
|---|---|---|---|
| M0 基线与契约 | 设计稿、现状测试、能力定义、版本锁定与环境检查 | 一键运行现有 Python/Node Skill，保存标准结果 | 旧 Skill 输入、依赖、四种输出、Artifact、错误语义形成基线 |
| M1 资源控制面 | 新资源表、状态机、幂等、租约、回收任务、Fake Backend | 创建 Binding、启动模拟 Job、重复删除、模拟崩溃恢复 | 并发创建唯一、跨租户拒绝、旧 generation 拒绝、清理重试 |
| M2 本地执行闭环 | Local Worker、gVisor、Runtime Agent、真实 Job/PTY/文件 | 命令生成文件；等待、输入、取消；停止计算后文件仍在 | 主机隔离、网络关闭、超时、资源上限、进程树清理、日志截断 |
| M3 E2B 后端 | Go REST Adapter、专用模板、认证数据面、暂停恢复 | 同一请求分别在本地与 E2B 执行，结果和错误结构一致 | 共用契约测试；429、超时、资源丢失、重复创建和孤儿回收 |
| M4 Agent/Skill 接入 | RuntimeTool、环境配置、发布快照、会话绑定、前端状态 | Agent 读取 Skill、运行脚本、跨工具调用复用文件、导出下载卡片 | 开关隔离、版本固定、用户停止、前端断开、旧 Skill 零行为变化 |
| M5 持久化与模板 | 对象存储、检查点、七天保留、HomeSnapshot、重建流程 | 安装环境后保存模板；新会话复用；释放计算后恢复文件 | 无业务文件混入模板、内部链接安全、保留期回收、快照失败不丢源数据 |
| M6A 受控联网 | 网关、策略绑定、授权撤销、网络探测 | 批准 HTTP 请求成功，私网和未授权请求失败 | TCP/IPv6/代理绕过/DNS 重绑定/重定向回归全部通过 |
| M6B 依赖自由度 | 用户空间准备、隔离构建、环境版本、依赖冲突处理 | 无 manifest Skill 安装 Python/Node 依赖并复用；系统依赖形成新模板 | 本地包不误装、离线失败清晰、失败可重试、构建不执行在控制面宿主机 |
| M6C 平台 CLI | 文件、模型和已绑定工具 Stub；审批集成 | 脚本上传产物、调用授权模型或工具 | 短令牌、审批暂停、撤权、跨租户拒绝、无长期密钥泄露 |
| M7 自建 VM | Kata Worker、节点检查、容量调度、卷恢复 | 同一 Agent 在 local/E2B/Kata 三种 Profile 运行同一任务 | 共用契约、KVM 隔离、节点失联、VM 清理、真实硬资源限制 |
| M8 生产化 | 指标、成本、备份恢复、灰度、运维手册 | 配额压测、故障演练、关闭新能力后旧 Skill 继续运行 | 无孤儿资源、恢复演练、账单对账、旧回归全绿、回滚可操作 |

### M0：先固定可回归基线

- 验证现有测试能运行，记录环境依赖和已存在的失败。
- 固定现有 Python/Node 示例输出与 Artifact 校验。
- 建立 Runtime 后端契约测试，后续所有后端复用。
- 增加 `runtime doctor`：检查隔离、网络、资源限制和环境版本。
- 固定实际验证过的镜像 digest、runsc、containerd、Kata 和 E2B 模板版本；不使用浮动 `latest`。

**退出条件：** 基线可重复运行，所有已知差异有记录，测试失败不会被新功能掩盖。

### M1：先证明生命周期，而不是先接模型

- 新表使用版本化、可重复执行的迁移。
- 保留旧 Sandbox bootstrap 和旧数据，不进行强制转换。
- 创建资源采用“持久意图＋幂等外部操作＋补偿回收”。
- 使用 Fake Backend 故障注入覆盖创建中途失败、数据库写入失败和清理失败。

**退出条件：** 任何失败都能归类为可重试、终态或待清理，没有不可追踪的资源。

### M2/M3：先实现同一个任务在两种环境运行

两个阶段在 M1 契约稳定后可并行，但验收独立。

固定演示：

1. 上传输入文件。
2. 启动 Python 或 Node Job。
3. 分批读取输出。
4. 产生 `artifacts/result.json`。
5. 下载并校验内容。
6. 取消长任务，确认后代进程退出。
7. 释放计算，再恢复环境。
8. 删除 Binding，确认物理资源被清理。

**退出条件：** 同一测试套件通过；只有供应商特有能力允许明确跳过，核心能力不得跳过。

### M4：形成第一个用户可见双后端 MVP

- 开启环境的控制台 Agent 获得新工具。
- 普通 `SKILL.md` 包可以通过 Shell 工作流执行。
- 原有 Script Skill 可以继续走旧模式，也可显式绑定到会话环境。
- 前端展示环境状态、Job 状态、停止操作和标准下载卡片。
- 暂不增加完整终端、IDE 或公开环境端口。

**退出条件：** 用户完成一次真实文件处理任务，无需手动调用 Sandbox API。

### M5：交付可靠的七天保留

- M3/M4 的短期暂停不等于七天持久化完成。
- 通过检查点、保留期和恢复测试后，才对外声明七天保留。
- 模板创建使用独立准备环境；不得直接把带业务数据的会话完整 VM 当作干净模板。
- 用户明确重建环境时，保留 Workspace 文件；依赖和进程按模板重新初始化。
- M5 的安装演示使用已批准的预构建依赖产物；任意用户空间联网安装由 M6A/M6B 交付。

**退出条件：** 控制面重启、计算资源销毁和恢复后，文件内容一致，模板中没有会话业务文件。

### M6：按三个小版本逐步提高自由度

先网络，再依赖，最后 CLI，不能一次性全开放。

- M6A 证明网络策略不能被用户进程绕过。
- M6B 在该网络边界内支持安装；系统构建与生产执行节点隔离。
- M6C 在既有授权体系中提供平台调用。

**退出条件：** 每新增一种自由度，都有对应的越权、撤权、失败和旧模式回归用例。

### M7/M8：自建 VM 与正式上线

- Kata 接入不修改模型工具和 Skill 契约。
- 首期采用节点粘性与检查点恢复，不实现透明热迁移。
- 本地节点无法访问时返回恢复状态或明确错误，不能静默切换到 E2B 并上传用户文件。
- 灰度完成后再开放更大的组织额度。

**退出条件：** 三后端通过公共核心用例，运行资源可对账、可清理、可恢复。

## 7. 验证、回归与 CI

### 7.1 四层验证

#### 第一层：纯 Go 单元与契约测试

每个 PR 执行：

- 资源状态机、权限、幂等、generation 和配额。
- Fake Backend 全故障路径。
- Job 输出、取消、退出码和游标。
- Snapshot 文件规则和错误分类。
- Agent 工具注册、开关、配置与发布快照。
- 旧 Script Skill 契约。

#### 第二层：真实本地后端

专用 Linux CI：

- 实际 containerd + gVisor。
- 文件、Job、PTY、输入、超时和取消。
- 访问宿主机、跨 Binding、网络绕过和资源耗尽。
- Worker 重启、孤儿回收和检查点恢复。

`preview-process` 不作为安全验收替代品。

#### 第三层：真实云和 VM

- E2B 使用隔离测试账户、费用上限和强制 finally 清理。
- Kata 使用具备 KVM 的专用 Runner。
- 供应商 Mock 测试不能替代真实创建、暂停、恢复和网络限制测试。
- 无凭据时测试可以显式跳过，但发布门禁要求有最近的真实通过记录。

#### 第四层：Agent/UI 端到端

- 使用固定任务和确定性断言，不只检查模型回答“成功”。
- 校验真实 Job、退出码、文件哈希和下载事件。
- 测试停止、重新生成、审批、前端断开、会话恢复和跨用户拒绝。
- 浏览器测试使用 `ZGI_E2E_EMAIL`、`ZGI_E2E_PASSWORD`，不硬编码凭据。

### 7.2 必须保留的旧回归

- Python 和 Node 示例。
- 有/无 manifest、有/无本地包声明。
- 本地包识别和第三方依赖授权。
- 扫描器开关。
- 单文件、多文件输入。
- `stdout_json`、`stdout_text`、`artifacts`、`mixed`。
- stderr、非零退出码和模型文件消息。
- 64 MiB Skill 包上限。
- Agent 未开启环境时没有新增工具。
- Workflow Code、插件 Runner 和公共 Agent 入口行为不变。

### 7.3 测试命令

沿用现有入口：

```text
make test-sandbox-kest
make test-api-skill-script-e2e
make -C sandbox test-linux-secure
```

相关 Go 模块执行：

```text
go test ./...
go vet ./...
```

新增阶段性入口（以下为待实现命令，不是现有可用入口）：

```text
make test-agent-runtime-contract
make test-agent-runtime-local
make test-agent-runtime-e2b
make test-agent-runtime-kata
make test-agent-runtime-e2e
make test-agent-runtime-recovery
```

涉及前端时执行现有 lint、TypeScript 检查、相关回归脚本和 build。生成步骤不得意外改动无关受跟踪文件。

### 7.4 每阶段交付证据

每个阶段必须提供：

- 可复现的演示步骤。
- 自动化测试入口和结果。
- 新旧行为差异。
- 故障注入结果。
- 资源清理结果。
- 配置与部署说明。
- 回滚操作及验证结果。

只有代码、没有真实演示和回归证据，不算阶段完成。

## 8. 节奏、灰度与回滚

### 8.1 推进节奏

估算按两名 Go 后端、一名平台工程师，前端和 QA 阶段性投入。不是交付承诺，M0 环境验证后修订。

| 工作 | 预计耗时 |
|---|---|
| M0 | 0.5–1 周 |
| M1 | 1.5–2 周 |
| M2 / M3 | 各 2–3 周，可部分并行 |
| M4 | 2–3 周 |
| M5 | 2–3 周 |
| M6A | 1–2 周 |
| M6B | 2–3 周 |
| M6C | 1–2 周 |
| M7 | 3–4 周，可与部分 M6 工作并行 |
| M8 | 2 周 |

预期：

- **首个用户可见本地＋E2B MVP：约 8–12 周。**
- **包括持久化、受控扩展、自建 Kata 和生产化：约 18–26 周。**
- 总投入约 **55–80 人周**；安全与恢复验证占明显比例。

每周检查实际 MVP，不以文件数量或代码行数衡量完成度。

### 8.2 PR 与分支

实施时：

- 从当时最新远端 `main` 创建 `px/agent-runtime-foundation`。
- 不直接在现有测试集成分支堆叠新架构。
- 按阶段拆分功能 PR；隔离控制面、后端、API 和 UI 修改。
- 每个 PR 附带阶段验收证据。
- 对旧分支差异进行只读比较，不自动合并或部署。
- 设计文档交付不包含服务器部署和远端分支变更。

### 8.3 灰度

采用独立开关：

- 新执行环境总开关。
- 各后端启用开关。
- 组织允许的 Profile/Policy。
- 用户空间安装、受控联网、CLI 分别授权。

上线顺序：

1. 内部测试组织。
2. 小规模白名单组织。
3. 有配额和预算限制的普通组织。
4. 扩大额度与后端选择。

### 8.4 回滚

- 关闭新环境工具与新 Binding 创建，不影响旧 Script Skill。
- 已有 Job 按运行策略结束或停止，不能删除用户文件作为回滚手段。
- 暂停、导出、删除和回收接口继续保留。
- 新数据库迁移以增量兼容为主；回滚应用不执行破坏性删表。
- Profile 按不可变版本管理，旧 Binding 仍可识别旧版本。
- 同时发布版本至少兼容相邻一版控制面和 Runtime Agent 协议。

### 8.5 运行指标

必须覆盖：

- 创建、恢复、暂停、删除成功率和时延。
- Job 队列等待、运行时间、退出原因和取消时延。
- Worker 心跳、活跃容量和租约续约失败。
- 孤儿资源数量、清理失败年龄。
- 检查点成功率、恢复成功率、存储使用量。
- 依赖构建成功率、缓存命中和失败分类。
- 网络拒绝、权限拒绝和审批等待。
- E2B 计算时长、存储和组织预算。

建立预算熔断；费用超限停止新建计算，不删除已有持久文件。

## 9. 最终验收与明确限制

### 完成标准

完整方案完成时，应能证明：

1. 同一个 Agent 与 Skill，在本地、E2B、Kata 上运行同一套任务。
2. 普通带脚本 Skill 不必新增 manifest 才能执行。
3. 本地包、用户空间依赖、系统依赖有不同且明确的处理路径。
4. 环境可释放计算、保留文件、恢复、重建和可靠删除。
5. 模型收到有界输出，前端收到真实文件下载卡片。
6. 用户停止、权限撤销、Worker 崩溃和供应商失败均有可回归行为。
7. 用户代码接触不到宿主机管理接口和长期平台密钥。
8. 旧 Skill、Workflow Code、插件 Runner 和未开启环境的 Agent 行为保持兼容。
9. 每个阶段都有独立 MVP，可以暂停推进而不留下只能半运行的主链路。
10. 部署、开发、运维、故障恢复和 Skill 兼容说明齐备。

### 不作出的承诺

- 不保证任意 Linux 软件都能在所有后端运行。
- 不保证依赖 GPU、特权内核能力、任意 TCP、桌面浏览器或 root 的 Skill 自动兼容。
- 不保证跨架构、跨基础镜像恢复安装目录。
- 不保证释放计算后后台进程继续存活。
- 不因为安装了 SSH 客户端就开放外部服务器访问能力。
- 不把关闭扫描器当作解决依赖缺失和权限限制的完整方案。

本次改造的目标是：**将 ZGI 从一次性脚本沙箱扩展为可管理、可恢复、多后端的 Agent 执行环境，同时保留现有受约束脚本链路，逐步提高自由度，而不是一次性解除全部安全边界。**

