# 数字人角色创建交互实施方案（对标示例产品·修订版）

日期：2026-09-15
版本：v2（修订同日 v1；v1 逐字副本保留在评审会话附件中）
状态：待确认第 15 节的 4 项开放决策后进入实施
范围：Web 前端创建流程；复用现有数字人创建接口，不新增后端接口
文件说明：文件名沿用 v1（`*-ops-specialist-create-interaction.md`）以便延续既有引用；v2 的范围已从「单一运维专员模板」扩展为「角色卡网格 + 自定义创建」。

---

## 0. 相对 v1 的修订摘要

| # | 维度 | v1 的写法 | v2 的修订 | 依据 |
| --- | --- | --- | --- | --- |
| 1 | 选择页形态 | 「推荐模板 + 空白创建」双卡，明确不排出模板市场 | 角色卡**网格** + 右上角「+ 自定义数字人」按钮 | 对齐示例产品的信息架构 |
| 2 | 角色数量 | 仅「运维专员」1 个正式模板 | 首发角色清单见 §7.2（对齐示例产品 10 张卡），原「运维专员」内容并入 DevOps 工程师 | 同上；避免与 DevOps 角色重复 |
| 3 | 能力区 | 4 行运维能力只读说明 | 保留只读说明，并显式给出「创建后进入能力配置」的落点与深链 | 示例产品在同页绑定，本期技术上不可行，见 D2 |
| 4 | 成功弹窗 | 仅「继续配置 / 返回数字人列表」 | 增加「接下来，选择一种方式开始协作」双卡（IM 分发 / 自主工作），主按钮仍为「继续配置」 | 对齐示例产品；但「直接对话」被既有契约阻断，见 D3 |
| 5 | 文件清单 | 8 新增 + 3 修改 | 9 新增 + 8 修改，补 `web/src/routes/console-navigation.ts`、`web/scripts/test-route-access.mjs`、`web/scripts/test-console-navigation-routes.mjs` | v1 漏项会直接导致路由/权限验证失败，见 §9 与 §12.3 |
| 6 | persona 规格 | 文字描述「模板 Persona + 用户职责上下文」 | 固定拼接模板 + 对应单测断言 | v1 未定义格式，无法写确定性测试 |
| 7 | 验证门 | 2 条命令 | 增加 `test:route-access`、`test:console-navigation-routes` | 新增路由必须同步路由清单 |
| 8 | 工作量 | 5—6.5 人日 | 8—11 人日（含 10 份角色内容）+ 角色头像设计资产 | 卡片数量与内容量变化 |

---

## 1. 方案结论

第一阶段交付一个可用、可验证的数字人创建闭环，交互形态对齐示例产品：

1. 用户从数字人列表点击「创建数字人」，进入独立创建页。
2. 创建页以角色卡网格展示预置角色，右上角提供「+ 自定义数字人」。
3. 选择角色后进入配置页：左侧编辑名称、头像与职责，右侧实时预览，底部固定操作栏。
4. 点击「创建为草稿」，通过现有创建接口写入预置的 `persona`、`goals`、`rules` 与模型，得到真实草稿。
5. 成功后弹出「选择一种协作方式」弹窗，两个入口分别指向 IM 分发与自主工作，并明确当前仍是草稿。

本阶段**不实现**服务器连接、命令执行、项目部署和环境诊断的真实能力，也不采集主机地址、密码、Token 或私钥。未具备的能力统一显示为「创建后接入」，不得出现模拟连接成功、模拟部署成功或可立即执行的暗示。这一条是 v1 的核心安全约束，v2 完整保留。

拆分依据：创建接口在资源绑定前必须先有数字人 ID（见 §4 证据 6），因此「一次表单配齐人设 + Skills + 知识库 + 连接器」的示例产品形态在现有接口下无法实现。本期先交付人设与职责闭环，能力绑定通过「创建后进入配置页」承接，不需要重做创建主流程。

---

## 2. 目标与非目标

### 2.1 目标

- 将列表页的简易创建弹窗升级为独立、可扩展的角色创建流程。
- 对齐示例产品的三段式形态：角色网格 → 配置页（表单 + 预览 + 底部操作栏）→ 成功弹窗。
- 使用现有 `CreateDigitalHumanRequest` 创建真实草稿，不做纯视觉假原型。
- 为每个预置角色提供有意义的 `persona`、`goals` 和 `rules`，避免得到只有名称和一句描述的空壳数字人。
- 保留「自定义数字人」，避免已有通用创建能力倒退。
- 给后续能力绑定、审批、执行记录和部署任务留下稳定扩展位置。

### 2.2 非目标

- 不新增或修改后端接口、数据表和权限模型。
- 不实现 SSH、堡垒机、云厂商、Kubernetes、CI/CD 或代码仓库连接。
- 不在创建请求中声明能力绑定、技能集合或连接器；不保存模板来源字段。
- 不实现「运行环境 / 本地设备」选择（ZGI 无对应模型，见 D4）。
- 不自动发布数字人；不允许未发布草稿进入对话（沿用既有契约）。
- 不改造现有「人设模板库」为用户自定义模板市场。

---

## 3. 差异与取舍决策台账

本节逐条记录「示例产品怎么做 / 本期怎么做 / 为什么」，避免实施期再次讨论。

### D1 选择页形态：网格 + 右上角自定义按钮

- 示例产品：10 张角色卡网格，卡片悬浮显示「查看详情」「创建」，右上角深色「+ 自定义 Waker」按钮。
- 本期：采纳网格形态。网格从只有 1 个角色时就按 `grid-cols-[repeat(auto-fill,minmax(...))]` 实现（沿用 `digital-human-list.tsx:88` 既有栅格写法），避免后续改版；「自定义」做成右上角按钮而非卡片。
- 理由：形态与首批内容量解耦，内容可分批补，不必等 10 个角色齐备才能上线。
- 差异：示例产品仅在悬浮时显示卡片操作。本期**额外**要求卡片可聚焦、Enter 进入创建、「查看详情」可键盘触达（`§11.3`）。这是本项目可访问性要求，属有意保留的差异。

### D2 能力绑定：留在创建后，不在创建页做可交互绑定

- 示例产品：创建页内直接展示 `Skills (6)` 芯片、知识库、连接器，且预置角色自带技能集（如项目管理员带 6 个技能 + atlassian 连接器）。
- 本期：创建页只展示**只读**能力说明与「创建后接入」状态；提交成功后引导用户到 `/console/digital-humans/{id}/configuration#configuration-capabilities` 完成绑定。
- 理由：能力绑定组件签名是 `DigitalHumanResourceBindings({ id }: { id: string })`（`digital-human-resource-bindings.tsx:109`），没有数字人 ID 无法渲染；现有接口也不支持「创建时一并绑定资源」。若强行在创建页做绑定，只能靠「创建成功后再串行补调用」，会引入部分失败状态（草稿已建、绑定失败），第一阶段的收益不足以承担这个复杂度。
- 差异：本期**不预置角色的技能集与连接器**。若产品坚持「选中项目管理员即自带 6 个技能」，需要后端支持创建时绑定，或前端接受串行调用 + 部分失败补偿，二者都超出本期范围。列为开放项 O2。

### D3 成功弹窗：采纳「选择协作方式」双卡，但不做「直接对话」

- 示例产品：弹窗主按钮是「直接对话」，主区是「在 IM 中 @Waker」与「设置自主工作」两张卡。
- 本期：保留「接下来，选择一种方式开始协作」双卡，导航落点改为真实存在且对草稿开放的页面：
  - 「在 IM 中 @数字人」→ `/console/digital-humans/{id}/channels`；文案必须说明需先完成 IM 应用配置（`channels` 页需要渠道凭证，未配置时是空态）。
  - 「设置自主工作」→ `/console/digital-humans/{id}/configuration#configuration-behavior`（触发策略区）。
  - 主按钮：「继续配置」→ `/console/digital-humans/{id}/configuration`；次按钮：「返回数字人列表」。
- 理由：未发布草稿不能进入对话，这是既有产品契约——`[digitalHumanId]/layout.tsx` 的 `isInitialSetup` 会把未发布数字人重定向到配置页，`chat` 路由还要求 `digital_human.run`（`scripts/test-digital-human-console.mjs:273-281` 正是保护这条不变量）。
- 差异：不提供「直接对话」「开始部署」「连接服务器」。若产品要求「直接对话」，需先决定「允许未发布草稿进对话」或「创建后自动发布」（后者要满足现有发布校验），属独立决策，列为开放项 O3。

### D4 运行环境：本期不做，且界面不得暗示

- 示例产品：配置页有「运行环境：本地 / 当前设备 / 切换运行环境」，右侧预览卡带「本地」徽标。
- 本期：不实现。创建页与预览**不得出现**「本地」「设备」「在线」「已连接」等字样。
- 理由：在 `web/src` 与 `api/internal` 中均未找到运行环境/设备的数据模型或接口（仅有无关的 i18n 文案命中，如 `channels` 的「按本地运行时接入」）。这是一个独立功能，且需要后端支持，不适合塞进本次前端交互改造。
- 差异：预览卡只显示「草稿」状态徽标与创建日期，不显示运行位置。

### O5 角色头像资产

- 示例产品：每个角色有独立的插画头像（黑发人物风格）。
- 本期：仓库现有可复用资产只有 6 张应用头像（`web/public/images/app-avatars/`：dolphin、jellyfish、little-z、manta-ray、octopus、whale），没有角色插画集。
- 默认方案：一期每个角色使用 `IconInput` 的文字头像（角色名首字/缩写 + 预设背景色），保证不依赖设计交付即可上线；若设计能交付角色插画，则替换为 `icon_type: 'image'`，创建请求字段不变，属纯资产替换。
- 需确认：是否为本期排入设计资源。

---

## 4. 当前约束（逐条已核对，附代码位置）

| # | 约束 | 证据 |
| --- | --- | --- |
| 1 | 当前创建入口是列表页弹窗，收集名称、用途、头像，成功后跳转配置页 | `web/src/components/digital-human/digital-human-list.tsx:193-324`，跳转见 `:259` |
| 2 | 现有请求默认值：`autonomy_level: 'supervised'`、`visibility: 'workspace'`、`memory_policy: {enabled:false, ownership:'digital_human'}`、`trigger_policy: {manual:true}`、`persona = 用途`、`goals: []`、`rules: []` | `digital-human-list.tsx:233-256` |
| 3 | 创建接口字段齐全，无需后端改动 | `web/src/services/types/digital-human.ts:124-140` |
| 4 | 权限码 `digital_human.create` 存在，且隐含 `digital_human.view` | `web/src/constants/permissions.ts:194`、`:446` |
| 5 | 存在可复用的权限门禁组件 | `web/src/components/digital-human/feature-gate.tsx:9-15`（`permission` 参数） |
| 6 | 资源绑定组件必须已有数字人 ID，创建前无法渲染 | `web/src/components/digital-human/digital-human-resource-bindings.tsx:109` |
| 7 | 必须存在可用的 Agent Runtime 模型才能创建 | `digital-human-list.tsx:203-211`、`:298-302`；`useAvailableModels({ use_case: 'agent-runtime' })` |
| 8 | 新建数字人是草稿；未发布数字人被强制重定向到配置页 | `scripts/test-digital-human-console.mjs:273-281`；`[digitalHumanId]/layout.tsx` 的 `isInitialSetup` |
| 9 | 配置编辑器是单页多锚点结构，可用 hash 深链 | `digital-human-configuration.tsx:344`(`#configuration-identity`)、`:455`(`#configuration-behavior`)、`:505`(`#configuration-capabilities`)；`digital-human-configuration-hub.tsx:9-16` 支持 `?section=behavior` 旧深链 |
| 10 | 头像交互组件的真实名称是 `IconInput`，不是 `IconPicker` | `web/src/components/common/icon-input/index.tsx`、`types.ts`（`IconValue` / `createTextIconValue`） |
| 11 | 列表卡片已有统一的图标展示工具 | `web/src/components/agents/agent-runtime/utils.ts` 的 `getAgentTextIconDisplay`，配合 `IconPreview` |
| 12 | i18n 模块 `digitalHumans` 已注册（`zh-Hans.ts` / `en-US.ts`）；`type-check` 会跑 `check-i18n-route-modules.mjs` 校验归属 | `web/src/i18n/modules/digitalHumans/`；`web/package.json` 的 `type-check` |
| 13 | 新增 `src/app/console/**` 页面会自动进入路由清单校验 | `web/scripts/test-route-access.mjs:1361-1460`（实际路由树与期望清单 `deepEqual`） |
| 14 | 精确路由优先于动态路由 | `web/src/routes/console-navigation.ts:589-590`（先查 exact map，`:602` 才查 dynamic） |

---

## 5. 信息架构与路由

统一使用一个独立路由，用查询参数表达当前角色，便于刷新、分享和浏览器返回：

| URL | 页面状态 | 说明 |
| --- | --- | --- |
| `/console/digital-humans/create` | `select_role` | 角色卡网格 + 自定义入口 |
| `/console/digital-humans/create?role=ops-engineer` | `configure` | 配置指定角色 |
| `/console/digital-humans/create?role=custom` | `configure` | 自定义创建 |

创建成功后不改变当前 URL，先展示成功弹窗并保存返回的数字人 ID。用户选择下一步后再导航：

- 「继续配置」：`/console/digital-humans/{id}/configuration`
- 「在 IM 中 @数字人」：`/console/digital-humans/{id}/channels`
- 「设置自主工作」：`/console/digital-humans/{id}/configuration#configuration-behavior`
- 「返回数字人列表」：`/console/digital-humans`

页面状态机：

```text
select_role
    ├─ 选择角色 ────→ configure(role)
    └─ 自定义 ─────→ configure(custom)

configure
    ├─ 返回选择 ───→ select_role
    ├─ 提交失败 ───→ error（保留用户输入）
    └─ 创建成功 ───→ created_draft（成功弹窗）

created_draft
    ├─ 继续配置 ───→ configuration
    ├─ IM 分发 ────→ channels
    ├─ 自主工作 ───→ configuration#configuration-behavior
    └─ 返回列表 ───→ list
```

### 5.1 路由注册（必做，v1 漏项）

新增 `web/src/app/console/digital-humans/create/page.tsx` 会自动被 `test-route-access.mjs` 的文件系统扫描发现，因此必须同步：

1. `web/src/routes/console-navigation.ts`：在 `ZGI_CONSOLE_SITE_MAP` 中新增**精确**路由条目
   `{ href: '/console/digital-humans/create', label: 'Digital Human Create', purpose: 'create a digital human from a role template', scope: 'workspace', permissions: ['digital_human.create'] }`。
   若不新增，`/console/digital-humans/create` 会命中动态规则 `/^\/console\/digital-humans\/[A-Za-z0-9_-]+$/`（`:391`），被当成「Digital Human Detail」并要求 `digital_human.view + digital_human.update + digital_human.publish`（`:396`）。由于 `digital_human.create` 只隐含 `view`（`permissions.ts:446`），**「有创建权限但无发布权限」的用户会被误判无权访问**。精确路由优先（`:589-590` 早于 `:602`），新增条目即可修复。
   **权限只写 `digital_human.create`**：`getZGIConsoleNavigationAccess` 用 `route.permissions.some(...)` 判定（`:668`），若同时列出 `view`，只看不建的用户也能打开创建页。
2. `web/scripts/test-route-access.mjs`：把 `/console/digital-humans/create` 加入 `expectedWorkspaceConsolePageRoutes`（`:1375` 起），否则该测试必然失败。
3. `web/scripts/test-console-navigation-routes.mjs`：补三条断言——`view + create` 放行、仅 `view` 拒绝、仅 `update + publish` 拒绝（防止回退到动态 detail 规则）。

---

## 6. 页面与交互规格

### 6.1 角色选择页（对齐示例产品）

- 标题「创建数字人」，副标题「选择一个适合工作场景的角色模板，或从空白开始创建」。
- 右上角主按钮「+ 自定义数字人」，始终可见（不依赖悬浮）。
- 主体为角色卡网格：`grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4`，与列表页栅格一致。
- 角色卡内容（自上而下）：角色头像/文字头像 → 角色名称 → 2 行截断简介 → 最多 3 个标签 chip。
- 卡片操作：「查看详情」「创建」两个按钮，悬浮显示、键盘聚焦时同样显示（`focus-within`）。
- 键盘与语义：卡片本体可聚焦，Enter 等同于「创建」；「查看详情」为独立可聚焦按钮，打开只读抽屉；不使用仅靠 hover 的交互。
- 只读抽屉内容：角色定位、职责描述、工作目标、安全规则、能力与接入状态、预置默认值；底部「使用此模板」。
- 空/异常态：角色清单为空（不应发生）时只显示「自定义数字人」入口并给出说明，不渲染空白页。

### 6.2 角色配置页（对齐示例产品）

桌面端为左侧表单、右侧实时预览、底部固定操作栏：

```text
┌ 返回 / 创建数字人 / <角色名> ─────────────────────────┐
│                                                        │
│  基本信息与角色内容                    数字人预览        │
│  ┌──────────────────────────┐          ┌────────────┐   │
│  │ 头像  名称               │          │ 头像/名称   │   │
│  │ 我负责什么               │          │ <角色名>    │   │
│  │ 角色能力与接入状态       │          │ 草稿        │   │
│  │ 高级设置：运行模型       │          │ 职责/标签   │   │
│  └──────────────────────────┘          └────────────┘   │
│                                                        │
├ 返回选择                    创建为草稿 ────────────────┤
```

#### 基本信息

- 头像：复用现有 `IconInput`（`showAvatarPresets`）与 `IconValue`、`createTextIconValue`；默认使用角色文字头像（角色名首字/缩写 + 角色预设背景色）。
- 数字人名称：必填，默认角色名称，沿用后端现有限制；编辑后同步更新右侧预览。名称变化时沿用现有弹窗的「首两字自动更新文字头像」行为（`digital-human-list.tsx:213-226`）。
- 我负责什么：必填，多行文本，预填角色职责文案，允许用户编辑。

#### 角色能力与接入状态

能力区的作用是解释角色边界，不是模拟配置成功。固定列表展示（每个角色 3—4 行，取自蓝图 `capabilities`）：

| 能力 | 用户价值 | 首期状态 |
| --- | --- | --- |
| 连接目标系统 | 在授权范围内访问目标环境 | 创建后接入 |
| 执行受控操作 | 查看服务、日志、进程与基础资源状态 | 创建后接入 |
| 交付与变更 | 按方案执行构建、发布与回滚 | 创建后接入 |
| 环境排障 | 收集证据、定位依赖与配置问题 | 创建后接入 |

- 能力行只显示说明与状态，不提供「连接」「测试」「立即部署」按钮。
- 区块底部提示：「草稿创建后可在配置页逐步绑定所需能力。请勿在对话或职责描述中填写密码、Token 或私钥。」
- 区块内提供明确的后续落点说明：创建完成后进入「配置 → 能力」区完成绑定（对应 `#configuration-capabilities`）。

#### 高级设置

- 默认折叠，标题旁展示当前模型名称。
- 展开后提供 Agent Runtime 模型选择器（`useAvailableModels({ use_case: 'agent-runtime' })`）。
- 第一个可用模型作为推荐值自动选中，但必须可见且可修改。
- 模型列表加载中禁用提交并显示加载状态；无可用模型时阻断提交并给出说明与现有模型管理入口（若已有稳定入口）。
- 注意既有契约：`scripts/test-digital-human-console.mjs:631-635` 断言**列表页源码**不得出现 `fields.persona|fields.rules|fields.model`（避免在创建弹窗里展开详细配置）。新创建页在独立文件中暴露模型选择不触发该断言，但仍须遵守其取向：创建页只暴露名称/头像/职责 + 模型，不暴露 persona/goals/rules 的逐项编辑。

#### 右侧预览

实时展示头像、数字人名称、角色名、草稿状态徽标、职责摘要与标签。只反映已写入创建请求的内容。**不得**显示「本地」「设备」「在线」「已连接」「技能已启用」等状态（见 D4）。

#### 底部操作栏

- 次按钮「返回选择」；存在未保存编辑时弹出确认对话框。
- 主按钮「创建为草稿」（有意保留「草稿」字样，明确当前语义；示例产品用「创建」，属可接受的文案差异）。
- 提交中按钮文案「正在创建…」，同时禁用重复提交与返回操作。
- 必填项错误落在具体字段下并聚焦首个错误，不能只用全局 Toast。
- 预留安全区，不遮挡最后一个表单字段。

### 6.3 自定义创建页

- 与角色配置页同路由、同组件，`role=custom` 时：
  - 不显示角色能力区与标签；
  - 不预填职责文案；
  - `persona` 直接来自用户填写的职责描述，`goals` 与 `rules` 保持空数组；
  - 其余默认值与当前弹窗完全一致（`autonomy_level: 'supervised'`、`visibility: 'workspace'`、`memory_policy`、`trigger_policy`），避免行为倒退。

### 6.4 创建成功弹窗（对齐示例产品）

- 标题：「<数字人名称> 已创建为草稿」（示例产品为「创建成功」，此处保留草稿语义）。
- 正文：一句自我介绍式说明（角色名 + 职责摘要），并说明「人设、工作目标与安全规则已写入；连接与执行能力仍需在配置页接入后才能使用」。
- 标签行：「我擅长」+ 角色标签（最多 4 个）。
- 分区标题：「接下来，选择一种方式开始协作」，两张卡片：
  - 「在 IM 中 @数字人」：说明 IM 群内分发；导航到 `/console/digital-humans/{id}/channels`；文案需注明「需先完成 IM 应用配置」。
  - 「设置自主工作」：说明按定时/事件/API 触发持续工作；导航到 `/console/digital-humans/{id}/configuration#configuration-behavior`。
- 主按钮「继续配置」；次按钮「返回数字人列表」。
- 不提供「直接对话」「开始部署」「连接服务器」（见 D3）。

---

## 7. 角色蓝图与 Persona 拼接规格

### 7.1 蓝图类型

第一阶段在前端定义只读蓝图，不创建新的后端模板实体：

```ts
export type DigitalHumanRoleId =
  | 'frontend-engineer'
  | 'backend-engineer'
  | 'qa-engineer'
  | 'product-manager'
  | 'data-analyst'
  | 'ui-designer'
  | 'content-operator'
  | 'devops-engineer'
  | 'project-manager'
  | 'community-qa';

export type DigitalHumanCapabilityStatus = 'requires_setup';

export interface DigitalHumanCapabilityTemplate {
  id: DigitalHumanCapabilityId;
  titleKey: string;
  descriptionKey: string;
  status: DigitalHumanCapabilityStatus;
}

/** 角色共用的能力文案目录，避免 10 个角色各写一份重复的能力描述 */
export const DIGITAL_HUMAN_CAPABILITY_TEMPLATES: readonly DigitalHumanCapabilityTemplate[];

export interface DigitalHumanRoleBlueprint {
  id: DigitalHumanRoleId;
  /** i18n key，文案统一走 digitalHumans 模块，保证中英一致 */
  titleKey: string;
  shortDescriptionKey: string;
  responsibilityKey: string;
  personaKey: string;
  /** 有序 i18n **叶子**键：顺序决定界面展示顺序与写入草稿的顺序 */
  tagKeys: readonly string[];
  goalKeys: readonly string[];
  /** 共享安全规则在前，角色专属规则在后 */
  ruleKeys: readonly string[];
  capabilityIds: readonly DigitalHumanCapabilityId[];
  capabilities: readonly DigitalHumanCapabilityTemplate[];
  /** 文字头像默认背景（十六进制色值，与 `icon_background` 同格式） */
  iconBackground: string;
  defaults: {
    autonomyLevel: 'supervised';
    visibility: 'workspace';
    memoryEnabled: false;
    manualTrigger: true;
  };
}

export const CUSTOM_ROLE_ID = 'custom';
export const DIGITAL_HUMAN_ROLE_BLUEPRINTS: readonly DigitalHumanRoleBlueprint[] = [/* ... */];

/** 文案解析由调用方注入；实现是一个纯函数 `(key: string) => string` */
export type RoleTextResolver = (key: string) => string;
```

约定：蓝图**只存 i18n key**，中文/英文文案分别落在 `web/src/i18n/modules/digitalHumans/zh-Hans.ts` 与 `en-US.ts`。不允许只写中文或让英文回退到中文键值。

**实现约束（勿踩）：i18n 消息树里不要用数组值。** 数组会把 `number` 泄漏进共享的 `DotNotationKeys` 联合（`web/src/i18n/translations.ts:33`），导致 `pnpm type-check` 在 `translations.ts:279/321` 报 `Type 'number' is not assignable to type 'string'`。因此 tags/goals/rules 一律用**具名叶子键**，由蓝图的 `tagKeys`/`goalKeys`/`ruleKeys` 提供顺序；这也避免数组下标位移导致既有译文错位。`scripts/test-digital-human-create-flow.mjs` 有一条回归断言专门拦截数组值重新混入。

### 7.2 首发角色清单

以示例产品的 10 张卡为基准（名称可在确认时调整）：

| 角色 | 简介要点（示例产品口径） | 标签示例 |
| --- | --- | --- |
| 前端工程师 | 前端界面设计与实现、组件架构、视觉语言、响应式与性能 | 前端设计 / 组件架构 / 响应式设计 |
| 后端工程师 | API 开发、数据建模、服务集成、性能优化与线上稳定性 | 代码审查 / 调试排障 / 架构 |
| 测试工程师 | 通用命令行与 Web 产品质量保障，测试设计、端到端验证、缺陷复现 | 测试计划设计 / 端到端验证 / 缺陷复现与分诊支持 |
| 产品经理 | AI 原生产品管理，目标驱动需求全生命周期、PRD 生成、用户反馈分析 | 需求全生命周期 / PRD 生成 / 竞品研究 |
| 数据分析师 | 问题框定、指标口径、数据收集、诊断与证据化建议 | 问题框定 / 指标口径 / 数据源审阅 |
| UI 设计师 | 设计体系下的界面设计与实现，界面诉求映射为组件方案 | 组件体系 / React UI 实现 / 设计交付 |
| 内容运营专员 | 账号定位、热点洞察、内容日历、笔记创作、素材 brief、发布与互动 | 账号定位 / 热点与选题规划 / 内容日历运营 |
| DevOps 工程师 | 偏交付与平台可靠性，CI/CD 流水线、基础设施自动化、环境管理、部署与回滚治理 | CI/CD 流水线设计 / 基础设施自动化 / 发布与回滚治理 |
| 项目管理员 | 范围澄清、里程碑规划、任务拆解、进度汇报、风险跟踪、干系人对齐 | 项目计划 / 进度与风险管理 / 跨职能协同 |
| 群聊答疑专员 | 在群聊中回答日常问题，参考已确认知识与群内上下文 | 自然对话答疑 / 证据化处理 / 上下文延续 |

- **原 v1 的「运维专员」内容并入 DevOps 工程师卡**（职责高度重叠，避免两张卡职责撞车）。若产品希望保留独立「运维工程师」卡，则为 11 张卡，清单相应调整。此条列入开放项 O1。
- 每个角色必须填齐：`persona`、≥3 条 `goals`、≥3 条 `rules`、≥3 个标签、3—4 项 `capabilities`。缺项不得合并。
- v1 已写好的运维 `Goals` / `Rules` 文案可直接复用到 DevOps 工程师卡，其余角色按同一结构产出。

### 7.3 Persona 拼接规格（v1 缺失，v2 固定）

用户只编辑名称、头像与职责；模板固定部分与用户职责的拼接方式必须确定，否则阶段 A 无法写确定性测试。固定为：

```ts
/** 注入式文案解析，保持本模块为纯函数、可脱离 React/i18n 运行时单测 */
export type RoleTextResolver = (key: string) => string;

export function buildPersona(
  blueprint: DigitalHumanRoleBlueprint,
  responsibility: string,
  t: RoleTextResolver
): string {
  const duty = responsibility.trim();
  return [
    t(blueprint.personaKey).trim(),
    '',
    '## 职责范围',
    `- 角色定位：${t(blueprint.titleKey)}`,
    `- 用户确认的职责描述：${duty}`,
    '- 若职责描述与角色定位冲突，以用户确认的职责描述为准，并在首次协作时先与用户核对。',
  ].join('\n');
}
```

- `t` 由调用方注入：页面传 `useT('digitalHumans')`，单测传一个读取 `zh-Hans.ts` 字典的查表函数。**禁止**在 `role-blueprints.ts` / `create-digital-human-payload.ts` 内直接 `import` i18n 运行时，否则阶段 A「不渲染页面也能测」的完成标准无法成立。
- 自定义创建：`persona = responsibility.trim()`，不追加模板段落。
- `description = responsibility.trim()`（与现弹窗一致，用于列表与预览摘要）。
- `identity_name = name.trim()`。
- 该函数的输出必须逐字进入创建请求，便于配置页展示与后续回归对比。

---

## 8. 创建请求映射

运维/角色创建请求由纯函数 `createDigitalHumanPayload(...)` 生成，方便单元测试并为未来后端模板接口保留替换边界：

| 请求字段 | 来源 |
| --- | --- |
| `name` | 用户填写的数字人名称 |
| `description` | 用户填写的职责描述 |
| `icon_type` / `icon` | `IconInput` 的 `IconValue`（文字头像按现弹窗序列化为 `{icon, icon_background}` JSON 字符串；图片头像取 `imageId`/`iconUrl`） |
| `identity_name` | 与数字人名称一致 |
| `persona` | `buildPersona(blueprint, responsibility)` |
| `goals` | 角色蓝图的 `goals`（自定义创建为空数组） |
| `rules` | 角色蓝图的 `rules`（自定义创建为空数组） |
| `default_model` | 高级设置中可见的模型选择 |
| `autonomy_level` | `supervised` |
| `visibility` | `workspace` |
| `memory_policy` | `{ enabled: false, ownership: 'digital_human' }` |
| `trigger_policy` | `{ manual: true }` |

能力清单与标签**不进入**创建请求：现有接口没有角色蓝图或能力声明字段。不得把能力状态塞进 `description`、`persona` 或其他非结构化字段伪装成已绑定资源。

---

## 9. 前端文件与职责（完整清单）

### 9.1 新增

| 文件 | 职责 |
| --- | --- |
| `web/src/app/console/digital-humans/create/page.tsx` | 路由入口；使用 `DigitalHumanFeatureGate permission="digital_human.create"`。注意这是**静态**路由（不同于 `[digitalHumanId]`），读取查询参数必须像 `console/dataset/page.tsx:451-456` 那样用 `<Suspense fallback={null}>` 包住读参内容，否则构建期会触发 `useSearchParams` 边界报错 |
| `web/src/components/digital-human/create/digital-human-create-flow.tsx` | 解析 `role` 参数，维护状态机、脏状态与创建成功 ID |
| `web/src/components/digital-human/create/digital-human-role-gallery.tsx` | 角色卡网格、只读详情抽屉、自定义入口 |
| `web/src/components/digital-human/create/digital-human-role-form.tsx` | 基本信息、能力说明、高级设置、字段级校验、底部操作栏 |
| `web/src/components/digital-human/create/digital-human-role-preview.tsx` | 无业务副作用的实时预览 |
| `web/src/components/digital-human/create/digital-human-create-success-dialog.tsx` | 草稿成功状态、「选择协作方式」双卡与导航 |
| `web/src/components/digital-human/create/role-blueprints.ts` | 蓝图类型、角色清单、共享能力目录与共享安全规则、默认值 |
| `web/src/components/digital-human/create/create-digital-human-payload.ts` | `buildPersona`、`serializeDigitalHumanIcon` 与「蓝图 + 表单值 → `CreateDigitalHumanRequest`」纯映射 |
| `web/src/components/digital-human/create/use-role-texts.ts` | 把强类型的 `useT('digitalHumans')` 适配成 `RoleTextResolver`；动态 key 的类型断言只允许出现在这一处 |
| `web/scripts/test-digital-human-create-flow.mjs` | 创建流契约测试（**必做**，已同步 `package.json` 的 `test:digital-human-create-flow`） |

### 9.2 修改（含 v1 漏项）

| 文件 | 改动 | 是否必改 |
| --- | --- | --- |
| `web/src/components/digital-human/digital-human-list.tsx` | 「创建数字人」改为导航到新路由；删除旧创建弹窗，避免两套创建逻辑并存 | 必改 |
| `web/src/routes/console-navigation.ts` | `ZGI_CONSOLE_SITE_MAP` 增加 `/console/digital-humans/create` 精确路由与 `view + create` 权限（见 §5.1） | **必改（v1 漏项）** |
| `web/src/i18n/modules/digitalHumans/zh-Hans.ts` | 角色清单、表单、能力状态、成功弹窗、错误提示文案 | 必改 |
| `web/src/i18n/modules/digitalHumans/en-US.ts` | 同上英文；不得回退中文键值 | 必改 |
| `web/scripts/test-route-access.mjs` | `expectedWorkspaceConsolePageRoutes` 增加创建路由（`:1375` 起） | **必改（v1 漏项）** |
| `web/scripts/test-console-navigation-routes.mjs` | 增加创建路由的访问断言，防止回退到动态 detail 规则 | 建议改 |
| `web/scripts/test-digital-human-console.mjs` | 迁移「创建后进入配置页」等旧断言（详见 §12.3） | 必改 |
| `web/package.json` | 增加 `test:digital-human-create-flow` 命令（已加），不引入新测试框架 | 必改 |

### 9.3 资产（可选）

| 路径 | 说明 |
| --- | --- |
| `web/public/images/role-avatars/*` | 若设计交付角色插画则新增；不交付时一期用文字头像（见 O5） |

---

## 10. 实施分解与交付顺序

### 阶段 A：蓝图、Persona 规格与请求契约（0.5—1 人日）

1. 定义 `DigitalHumanRoleBlueprint` 与角色清单骨架。
2. 实现 `buildPersona` 与 `createDigitalHumanPayload` 纯映射。
3. 为默认值、persona 拼接（模板段 + 职责段）、角色 goals/rules 写入、自定义创建空值兼容编写测试。
4. 建立 i18n key 骨架（不要求最终文案）。

完成标准：不渲染页面也能通过测试证明「选中某角色 + 填名称职责」最终会创建什么数据；请求中不存在凭证字段或虚假能力绑定。

### 阶段 B：角色内容与资产（与 A 并行，1—2 人日，内容工作量为主）

1. 按 §7.2 产出 10 个角色的 persona、goals、rules、标签、能力行与中英文案。
2. 确认头像方案（文字头像或设计插画）。

完成标准：每个角色的蓝图字段完整无缺项，中英 key 一一对应。

### 阶段 C：路由与角色选择交互（1 人日）

1. 新增创建路由、权限门禁与 **`console-navigation.ts` 精确路由注册**。
2. 实现角色卡网格、只读详情抽屉与自定义入口。
3. 用 URL 查询参数驱动选择，验证刷新、前进、后退。
4. 修改列表页入口；同步更新 `test-route-access.mjs` 与 `test-console-navigation-routes.mjs`。

完成标准：能从列表进入创建页，用鼠标与键盘选择角色或自定义；无创建权限时看不到可操作表单；`pnpm test:route-access` 与 `pnpm test:console-navigation-routes` 通过。

### 阶段 D：配置、预览与状态反馈（1.5—2 人日）

1. 实现表单、能力说明区、高级模型设置与实时预览。
2. 实现字段级校验、加载态、无模型阻断态与脏数据返回确认。
3. 完成桌面与移动端布局（`lg` 双栏、小屏单栏 + 折叠预览）。

完成标准：所有控件状态、错误与能力边界清楚；页面不含不可兑现的连接/部署动作，也不出现「本地/设备/在线」字样。

### 阶段 E：真实草稿创建与成功闭环（1 人日）

1. 接入现有 create mutation；防止重复提交；失败保留表单并可重试。
2. 成功后展示成功弹窗与三个导航出口。
3. 自定义创建保持当前请求默认行为。

完成标准：创建的草稿在详情中具有非空人设、目标、规则与模型；成功后不误导用户直接使用未接入能力。

### 阶段 F：回归、契约迁移与可访问性（1—1.5 人日）

1. 按 §12.3 迁移既有断言，而不是删除。
2. 补充创建流聚焦测试。
3. 完成 TypeScript、ESLint、桌面/移动端与键盘回归。
4. 只调整本方案范围内的交互，不顺带改造配置页。

预计工作量：单名前端工程师 **8—11 人日**（较 v1 的 5—6.5 人日增加，主要来自 10 个角色的内容量与成功弹窗双卡）；另建议预留 1—2 人日产品与测试验收。角色插画若需设计交付，单独排期。

---

## 11. 异常、边界与可访问性

### 11.1 异常状态

- 模型加载失败：内联错误 + 重试按钮，禁用创建。
- 无可用模型：说明需先配置 Agent Runtime 模型，禁用创建。
- 创建接口失败：保留全部输入，表单顶部展示错误摘要，Toast 补充反馈。
- 网络中断或超时：恢复按钮可点击，允许重试，不自动创建第二个草稿。
- 返回角色选择：有编辑时二次确认；确认后清空未提交表单。
- 创建成功但导航失败：成功弹窗保留数字人 ID，允许再次点击导航。
- 无效 `role` 参数：回退角色选择页并显示一次轻量提示，不渲染空白页。

### 11.2 响应式

- `lg` 及以上为表单 + 预览双栏，预览在视口内保持可见。
- 小屏改单栏，预览移到表单下方或折叠为「查看预览」。
- 角色卡网格在小屏变为单列（`minmax(15rem,1fr)` 自动换行即可）。
- 底部操作栏预留安全区，不遮挡最后一个字段。
- 成功弹窗在小屏使用接近全屏的纵向布局，卡片与按钮保持易点击。

### 11.3 可访问性

- 卡片与按钮具有可见焦点，Enter/Space 能完成选择；悬浮才出现的操作在键盘聚焦时同样可见。
- 输入框有永久标签，错误通过 `aria-describedby` 关联。
- 详情抽屉与成功弹窗打开后管理焦点，关闭后返回触发控件。
- 状态不能只用颜色表达，同时显示「创建后接入」「草稿」文字。
- 提交状态通过 `aria-live` 告知，不依赖旋转图标。
- 尊重减少动态效果设置。

---

## 12. 验收清单

### 12.1 功能验收

- [ ] 有 `digital_human.create` 权限的用户可从列表进入独立创建页。
- [ ] 无创建权限的用户无法使用创建表单和提交接口；**只有 view + create、没有 update/publish 的用户也能访问创建页**（验证 §5.1 的精确路由注册）。
- [ ] 角色选择页包含角色卡网格与「+ 自定义数字人」两个入口，且卡片操作可用键盘触达。
- [ ] 角色详情抽屉能看到职责、目标、安全规则与能力接入状态。
- [ ] 名称、头像、职责变化会同步到右侧预览。
- [ ] 当前模型可见且可修改；模型未就绪时无法提交。
- [ ] 点击一次「创建为草稿」只发起一次创建请求。
- [ ] 创建请求包含非空 `persona`、`goals`、`rules` 和 `default_model`；persona 严格符合 §7.3 的拼接格式。
- [ ] 页面和请求都不采集密码、Token、私钥或其他凭证；页面不出现「本地/设备/在线/已连接」字样。
- [ ] 能力区不出现虚假的已连接、可执行、部署成功状态。
- [ ] 创建失败保留输入并可重试。
- [ ] 成功弹窗明确区分「已完成」与「待接入」，三个出口导航正确且对草稿开放。
- [ ] 自定义创建仍能完成真实草稿创建，且默认值与旧弹窗一致。

### 12.2 技术验证

在 `web/` 目录执行：

```bash
pnpm test:digital-human-console
pnpm test:route-access
pnpm test:console-navigation-routes
pnpm type-check
pnpm exec eslint <本次修改的 ts/tsx 文件>
```

若新增创建流独立测试，再执行：

```bash
pnpm test:digital-human-create-flow
```

注意：`test:route-access` 与 `test:console-navigation-routes` 当前**未纳入 CI 的 web 作业**（`.github/workflows/ci.yml:86-121` 只跑固定的若干脚本 + `--frontend-only` 的 eslint/type-check），但它们属于仓库既有验证约定，必须手动执行并通过。

人工回归至少覆盖：

- 1440px 桌面视口与 390px 移动视口。
- 仅键盘完成角色选择、填写与创建。
- 模型加载中、无模型、接口失败、重复点击、导航失败。
- 中文与英文界面。
- 创建后打开配置页，核对人设、目标、规则、模型与草稿状态。

### 12.3 既有契约断言的迁移（已按此实施）

| 断言 | 位置 | 处理方式 | 实施结果 |
| --- | --- | --- | --- |
| `list` 源码中必须出现 `router.push('/console/digital-humans/${id}/configuration')` | `test-digital-human-console.mjs:273-277` | 拆成两条：`list` 保留「已发布 vs 配置页」的卡片跳转，创建流接管「新建草稿 → 进入 configuration」 | 已迁移为 `list` 断言 + `createFlow` 断言（`draftBase/configuration`） |
| `list` 源码中不得出现 `fields.persona\|fields.rules\|fields.model` | `test-digital-human-console.mjs:631-635` | 保持，并新增一条针对创建表单的同类断言（可写模板人设/目标/规则，但不得暴露为可编辑字段） | 已实施 |
| `list` 与 `configuration` 的对话框必须使用 `DialogBody` | `test-digital-human-console.mjs:690-695` | 列表页不再拥有对话框，改为断言创建成功弹窗与配置页使用 `DialogBody`，并断言 `list` 不再渲染 `<Dialog` | 已实施 |
| `list` 与 `configuration` 必须使用 agent-runtime 模型候选 | `test-digital-human-console.mjs:165-179` | 模型加载移到创建流：改为断言 `configuration` 与 `createFlow`，并断言 `list` 不再引用 `useAvailableModels` | 已实施 |
| 未发布数字人路由必须重定向到配置页 | `test-digital-human-console.mjs:278-281` | 保持不变；成功弹窗不得提供绕过该重定向的入口 | 保持未改动（O3 决策） |

---

## 13. 后续能力接入边界

交互第一阶段验收通过后，再按以下顺序扩展，避免同时解决凭证、安全、执行与部署四类高风险问题：

1. 服务器连接资源：凭证托管、连通性测试、主机指纹确认、权限范围。
2. 只读诊断工具：系统信息、磁盘、内存、进程、服务状态与日志读取。
3. 受控执行：命令白名单/黑名单、风险分级、审批、超时、审计与结果证据。
4. 项目部署：项目来源、环境参数、部署计划、健康检查与回滚。
5. 自动化任务：定时巡检、告警触发、并发限制与失败升级。
6. 角色预置技能集与连接器（对应开放项 O2）：需要后端「创建时绑定」或前端串行补偿方案。
7. 运行环境 / 本地设备（对应 D4）：需要新的数据模型与接口。

届时保留本方案的角色选择、基础配置与成功闭环，仅将「创建后接入」能力行升级为真实资源选择器。若后端新增角色蓝图目录，`role-blueprints.ts` 与 `create-digital-human-payload.ts` 就是唯一需要替换的数据适配层。

---

## 14. 完成定义

只有同时满足以下条件，才视为「数字人角色创建交互」完成：

1. 用户可以从列表开始，在不接触任何凭证的情况下创建一个真实角色草稿。
2. 角色价值、当前可用范围、待接入能力与创建后的下一步在每个关键页面都清楚可见。
3. 草稿数据包含可用于后续配置的人设、目标、安全规则与模型，而不是只生成一个视觉卡片。
4. 自定义创建、权限门禁与既有数字人配置流程没有回归；§12.3 的三条既有契约断言全部以「迁移」而非「删除」的方式处理。
5. `test:digital-human-console`、`test:route-access`、`test:console-navigation-routes`、`type-check`、ESLint 全部通过，桌面、移动端、键盘与主要异常状态完成人工验收。

---

## 15. 待确认清单（阻断项）

| 编号 | 待确认 | 影响 | 建议默认 |
| --- | --- | --- | --- |
| O1 | 首发角色清单是否就是示例产品的 10 个角色；「运维专员」并入 DevOps 还是单列 | 阶段 B 内容量与卡片数量 | 采用 10 卡，运维内容并入 DevOps 工程师 |
| O2 | 预置角色是否需要自带技能集与连接器 | 若需要，必须改后端或接受串行补偿，工作量与风险显著上升 | 一期不做，创建后手工绑定 |
| O3 | 成功弹窗是否需要「直接对话」 | 需要先允许未发布草稿进对话或创建后自动发布，涉及既有契约 | 一期不做，用「继续配置」替代 |
| O4 | 是否为 10 个角色排入插画头像设计资源 | 无资产则一期用文字头像 | 一期文字头像，插画后续替换 |

上述 4 项确认前，可先行开工的只有阶段 A（蓝图类型、`buildPersona`、请求映射与单测）与 i18n key 骨架；阶段 C—E 建议在 O1、O4 确认后启动，O2、O3 若确认要做，需先补充方案再评估工期。

---

## 16. 实施记录

### 16.1 已完成（阶段 A—E + 阶段 F 的断言迁移）

| 项 | 产物 | 验证 |
| --- | --- | --- |
| 蓝图与角色目录 | `web/src/components/digital-human/create/role-blueprints.ts`：10 个角色、11 个共享能力模板、4 条共享安全规则、有序 key 列表 | 契约测试断言字段完整、能力状态恒为 `requires_setup`、共享规则必定写入每个角色 |
| Persona 与请求映射 | `create-digital-human-payload.ts`：`buildPersona`、`serializeDigitalHumanIcon`、`createDigitalHumanPayload` | 契约测试逐字校验 persona 拼接格式，并锁定请求字段白名单（13 个 key，无凭证字段、无能力声明） |
| i18n 文案 | `digitalHumans/{en-US,zh-Hans}.ts` 的 `createFlow` 子树（约 200 键/语言） | `pnpm type-check`（en-US 为类型源，中文缺键即报错）+ 测试断言两语言 key 形状一致、无空文案、无数组值 |
| 文案解析适配 | `use-role-texts.ts` | ESLint + type-check |
| 契约测试 | `web/scripts/test-digital-human-create-flow.mjs` + `package.json` 的 `test:digital-human-create-flow`：上半部分是**载荷契约**（字典驱动），下半部分是**UI 源码契约**（防重复提交、成功后不再建第二个草稿、失败保留输入、模型门禁、字段级错误与 aria、键盘可达、reduced-motion、文案不得宣称已连接/在线/已部署） | 退出码 0；并用变异验证确认断言非空洞（删掉 `if (submitting) return;` 后测试以 `submit must ignore re-entrant clicks` 失败，恢复后通过） |
| 创建路由 | `src/app/console/digital-humans/create/page.tsx`（静态路由 + `<Suspense>` + `DigitalHumanFeatureGate permission="digital_human.create"`）；`console-navigation.ts` 精确路由条目 | `test-route-access` + `test-console-navigation-routes` 通过 |
| 角色选择页 | `digital-human-role-gallery.tsx`：角色卡网格、键盘可达的「查看详情 / 创建」、只读详情抽屉、右上角「+ 自定义数字人」 | 源码契约断言 + ESLint |
| 配置页与预览 | `digital-human-role-form.tsx`（基本信息 / 只读能力区 / 高级模型设置 / 字段级校验 / 固定底部操作栏）、`digital-human-role-preview.tsx`（实时预览，不显示设备或在线状态） | `create form` 被断言不得暴露 persona/goals/rules 编辑 |
| 成功闭环 | `digital-human-create-flow.tsx`（URL 驱动 + 按角色 keyed 状态 + create mutation + 脏数据确认）与 `digital-human-create-success-dialog.tsx`（已完成/待接入 + IM/自主工作双卡 + 继续配置） | 断言「创建后进入 configuration」、「只用 create mutation、不 update/publish」 |
| 列表页入口 | `digital-human-list.tsx` 改为链接到创建路由，删除旧创建弹窗 | `test-digital-human-console` 断言列表不再含 `useAvailableModels` 与 `<Dialog` |

已决策：O1 采用 10 卡并把原「运维专员」内容并入 DevOps 工程师；O2 一期不做（创建页只读说明 + 创建后绑定）；O3 一期不做（成功弹窗主按钮为「继续配置」）；O4 一期用文字头像（`iconBackground` 为十六进制色值，与 `icon_background` 同格式）。

### 16.2 剩余（仅浏览器人工验收）

阶段 A—F 的代码、断言迁移与全部自动化验证均已完成（4 个测试套件、`tsc`、ESLint、i18n 路由模块检查、生产构建）。

仅剩**需要运行环境的人工验收**（本机无可用前端+后端+浏览器环境，无法执行）：

- 1440px 桌面 / 390px 移动视口下的布局与底部操作栏安全区。
- 纯键盘完成「进入创建页 → 选择角色 → 填表 → 创建 → 成功弹窗」全流程。
- 模型加载中 / 无可用模型 / 创建接口失败 / 重复点击 / 导航失败的异常态。
- 中英文界面互查，以及创建后在配置页核对人设、目标、规则、模型与草稿状态。
- 真实接口联调（`test:digital-human-create-flow` 用的是字典与源码契约，不代表后端联调）。

已在本轮排除的两个静态疑点：①「页面是否需要自己滚动」——`DefaultConsoleShell` 的 `<main>` 已带 `overflow-auto`（`customer/default.tsx:180-184`），页面无需再包滚动容器；②「减少动态效果」——`globals.css` 的三个 `prefers-reduced-motion` 块只覆盖 logo 与 workflow 面板，因此新增组件统一用 `motion-reduce:transition-none`（与 `resource-sidebar.tsx`、`console-sidebar.tsx` 的既有做法一致），并由契约测试计数锁定。

### 16.3 实施期发现（已回写到本文档）

1. **i18n 消息树不能用数组值**（约束见 §7.1）。首版把 tags/goals 写成数组，`pnpm type-check` 在 `translations.ts:279/321` 报 `Type 'number' is not assignable to type 'string'`——数组把 `number` 泄漏进共享的 `DotNotationKeys` 联合。已改为具名叶子键 + 蓝图有序 key 列表，并在契约测试中加了「消息树不得出现数组」的回归断言。baseline 对照确认该报错由本次改动引入（还原两个 i18n 文件后 `tsc` 退出码 0）。
2. **`role-blueprints.ts` / `create-digital-human-payload.ts` 只能有 type-only import**。node 的类型剥离不做无扩展名解析，一旦出现运行时跨模块 import，`test:digital-human-create-flow` 就无法加载它们。同理，共享安全规则由蓝图携带（`ruleKeys` 已包含共享项），payload 模块不 import 蓝图模块。
3. **动态 i18n key 的类型断言集中在 `use-role-texts.ts`**，与 `app/console/workspace/page.tsx` 的既有做法一致，蓝图与 payload 模块保持纯粹可测。
4. **scoped 翻译器的 key 必须带容器前缀**。`useT('digitalHumans')` 的 key 类型是「去掉模块名后的完整路径」，所以 `createFlow` 下的 `form.*` / `detail.*` / `preview.*` / `success.*` / `capabilityStatus.*` 在组件里必须写成 `createFlow.form.*` 等；写成 `t('form.name')` 会在 type-check 报 TS2345。本次共修正 56 处调用点。
5. **`test:route-access` 在改动前就是红的**：基线的 `expectedWorkspaceConsolePageRoutes` 漏了 4 个已提交路由（`:digitalHumanId/chat`、`:digitalHumanId/external`、`.../external/docs`、`.../external/keys`，其中 chat 页由提交 `2909dd634` 引入）。本次一并补齐，否则新增路由后仍无法通过。
6. **路由权限用 `some()` 判定**（`console-navigation.ts:668`），因此创建路由的权限列表只能写 `digital_human.create`。
7. **滚动容器由 shell 提供**：`DefaultConsoleShell` 的 `<main>` 已是 `overflow-auto`（`customer/default.tsx:180-184`），页面组件不需要（也不应）再套一层 `h-full overflow-y-auto`；`sticky bottom-0` 的操作栏以 `<main>` 为滚动祖先正常工作。
8. **`prefers-reduced-motion` 要按组件显式处理**：`globals.css` 的全局块只覆盖 logo 与 workflow 面板，不包含通用 transition；仓库约定是 Tailwind 的 `motion-reduce:transition-none`（`resource-sidebar.tsx:171`、`console-sidebar.tsx:457` 等）。新增组件的 4 处 transition 已全部补上，并由契约测试断言「transition 数量 == motion-reduce:transition-none 数量」。

### 16.4 环境注意

在本机沙箱内执行 `pnpm` 会在仓库根生成未被 `.gitignore` 覆盖的 `.pnpm-store/`（沙箱写不了用户级 store），提交前必须删除；直接调用 `node node_modules/typescript/bin/tsc --noEmit` 等可避免产生该目录。

### 16.5 本地 Docker 部署记录

本次把改动部署到了本机默认栈（`COMPOSE_PROJECT_NAME=zgi`，web 端口 2680），只重建 web 服务：

```bash
# 1) 基础镜像从可达的镜像源取，并重命名成 Dockerfile 期望的名字
docker pull docker.m.daocloud.io/library/node:20-alpine3.20
docker tag  docker.m.daocloud.io/library/node:20-alpine3.20 node:20-alpine3.20
docker pull docker.m.daocloud.io/docker/dockerfile:1
docker tag  docker.m.daocloud.io/docker/dockerfile:1 docker/dockerfile:1

# 2) 构建（RUN 步骤的 apk/npm 走宿主机代理）
docker compose --progress plain -f docker/docker-compose.yaml --env-file docker/.env build \
  --build-arg HTTP_PROXY=http://host.docker.internal:10808 \
  --build-arg HTTPS_PROXY=http://host.docker.internal:10808 \
  --build-arg NO_PROXY=localhost,127.0.0.1 web

# 3) 只重建 web 容器
docker compose -f docker/docker-compose.yaml --env-file docker/.env up -d --no-deps web
```

为什么要绕：本机 `registry-1.docker.io` 被 DNS 污染（解析到 `104.244.43.182`），本地没有任何基础镜像缓存，Docker VM 内 `dl-cdn.alpinelinux.org` / `registry.npmjs.org` / Docker Hub 全部超时；宿主机只有 mixed 端口 `127.0.0.1:10808`（同时支持 SOCKS5 与 HTTP 代理协议），容器可经 `host.docker.internal:10808` 使用它。**不需要修改 Docker Desktop 配置或重启 Docker。**

同时修掉一个本地生成物缺陷：`docker/docker-compose.yaml`（gitignore 的生成物）原本把 web 的构建上下文写成 `context: ..`，而模板与 CI 都是 `../web`；仓库根没有 `package.json`，按旧文件构建会在 `COPY package.json` 直接失败。已用 `docker/generate_docker_compose.ps1` 重新生成。

验证证据：容器 `zgi-web-1` healthy；运行镜像 ID 与刚构建的 `zgi-web:latest` 一致（`sha256:31463049…`）；容器内存在 `.next/server/app/console/digital-humans/create`；`/console/digital-humans/create` 返回 200；客户端产物 `static/chunks/0anp.f35zy5ru.js` 含新文案「自定义数字人」。

回滚：`docker tag zgi-web:pre-role-flow zgi-web:latest && docker compose -f docker/docker-compose.yaml --env-file docker/.env up -d --no-deps web`（旧镜像已保留为 `zgi-web:pre-role-flow`）。

注意：后端 `zgi-api-1` 仍跑注册表里的 `zgiai/zgi-api:latest`（构建于 2026-09-08，未重建）。本次改动只在前端，创建请求字段与旧弹窗完全一致（仅 goals/rules 由空数组变为角色预置内容）；若验收时角色创建报错，优先怀疑该旧后端镜像，而不是本次前端改动。

### 16.6 端到端验证（真实接口，已执行）

用一个临时脚本登录本机 E2E 账号（凭据取自 gitignore 的 `.env.test.local`），**直接调用 `create-digital-human-payload.ts` 构造请求**并对运行中的后端跑了一遍完整链路。接口前缀为 `/console/api`（控制台 API 组，见 `api/routes/router.go:64`）：

| 步骤 | 接口 | 结果 |
| --- | --- | --- |
| 登录 | `POST /console/api/login` | 200，`code=0`，取到 access_token |
| 取模型 | `GET /console/api/llm/models/available?use_case=agent-runtime` | 200，1 个可用模型 `deepseek-chat` |
| 创建 | `POST /console/api/digital-humans` | **201，`code=0`**，返回 definition.id |
| 读回 | `GET /console/api/digital-humans/{id}` | `status=draft`、persona 261 字符且含「## 职责范围」、goals=4、rules=5、`default_model=deepseek-chat`、`memory_policy.enabled=false`、`trigger_policy.manual=true` |
| 清理 | `DELETE /console/api/digital-humans/{id}?expected_revision=…` | 200 success（探针草稿已删除，未在库中留下数据） |

结论与影响：

1. §11.1 的核心验收项「创建请求包含非空 `persona`、`goals`、`rules` 和 `default_model`」**已在真实后端上验证通过**，此前对「旧后端镜像可能不接受角色预置 goals/rules」的担心可以排除。
2. `POST /digital-humans` 经 `digitalHumanPermissionMiddleware` 映射到 `WorkspacePermissionDigitalHumanCreate`（`api/routes/v1/digital_human_routes.go:224`），因此返回 201 同时证明**该账号持有 `digital_human.create`**，验收时不会撞到创建页的权限门禁。
3. 观察（非本次缺陷）：后端在创建时**不校验** `default_model`，空模型也能建出草稿（首轮探测即如此）；前端创建表单按设计在无可用模型时禁用提交，发布前另有既有校验兜底。

### 16.7 移除数字人专属搜索 Skill（测试功能下线）

背景：`digital-human-exa-web-search` 是数字人早期的「专属搜索 Skill」测试功能，把 Exa 凭据直接挂在数字人上。后续搜索能力统一由**连接器（connector）**承担，因此按用户要求整体下线。

复用审计结论（先确认再动手）：

- `digital_human_direct_skill_credentials` 表与全部 `DirectSkill*` 符号只出现在 `api/internal/modules/digitalhuman/` 及其建表迁移中，**没有其它模块复用**。
- Skill id `digital-human-exa-web-search` 在仓库其它位置不再出现。
- 通用 Exa **连接器**集成（`api/internal/container/service_container.go:1065` 注册的 `exa_integration`）与本次无关，**已保留**：搜索改由连接器承担。

删除内容：

| 层 | 变更 |
| --- | --- |
| 后端 | 删除 `direct_skill_exa.go` / `direct_skill_model.go` / `direct_skill_repository.go` 及其测试；`run_handler.go`、`run_service.go`、`user_input_continuation.go`、`candidates.go`、`brain_adapter.go` 移除全部 direct-skill 注入与分支；`routes/v1/digital_human_routes.go` 去掉 `WithDirectSkillRuntime(...)` |
| 前端 | `digital-human-resource-bindings.tsx` 删除 Exa 配置面板（157 行）与相关未用导入/属性；`use-digital-humans.ts`、`query-keys.ts`、`digital-human.service.ts`、`services/types/digital-human.ts` 删除 Exa 配置 hook、query key、接口与类型 |
| 文案 | 两个语言包删除 `bindings.exa.*`（各 21 个 key），`createFlow.templates.webResearch.*` 与搜索模板一并删除 |
| 模板 | 只保留「文件生成助手」(`fileGeneration`)，绑定真实 Skill `file-generator`；契约测试同步为 1 个模板 + 真实 skill id 白名单 |

保留：建表迁移 `202608261100000000_add_digital_human_direct_skill_credentials.go`（含测试）。原因见下方约束。

验证（全部通过）：

- `go vet ./internal/modules/digitalhuman/ ./routes/v1/ ./internal/modules/llm/modelmeta/` = 0；改动文件 `gofmt -l` 为空。
- 前端 `tsc --noEmit` 干净；ESLint 对改动目录 0 error；`test:digital-human-console`、`test:digital-human-create-flow`、`test:digital-human-chat-route-sync`、`test:digital-human-api-docs`、`test:route-access`、`test:console-navigation-routes`、`check-i18n-route-modules` 全部通过。
- 真实接口 E2E（`api` / `web` 镜像已重建并滚动重启，容器 healthy）：创建 → `GET .../draft/binding-candidates?type=skill` **只返回 `file-generator`**（`digital-human-exa-web-search` 已消失）→ 绑定 `file-generator` → 发布（`status=active`）→ 下架（`status=disabled`）→ 删除（随后 `GET` 404）。E2E 数据已全部清理（列表剩余 0 条）。

约束与遗留（需要知道）：仓库的迁移检查器禁止 Up 迁移出现 `DROP TABLE`（`api/internal/migrations/check.go:345`，字符串字面量同样拦截），且已应用的迁移文件不能删除（`validateUnknownMigrations`）。因此**无法用合规迁移删除该表**：本地 `zgi` 库已手工 `DROP TABLE digital_human_direct_skill_credentials`（该表原本 0 行）。迁移文件保留意味着全新部署仍会建出这张空表——后续若要彻底清理，需要在放开策略后补一条 cleanup 迁移。

### 16.8 修复「数字人添加 Skill 弹窗只剩 file-generator」

现象：数字人配置页 → 能力 → 添加 Skill，候选从「一大批系统 Skill」掉到只剩 `file-generator`。

定位过程（真实接口 + 代码追溯）：

1. 组织技能配置里 **21 个 skill 全部启用**，但 `GET /console/api/digital-humans/{id}/draft/binding-candidates?type=skill` 只返回 1 条 → 限制不在组织启用，而在候选过滤器。
2. 根因是分支提交 `3df06a5af`（2026-09-14，仅存在于 `px/digital-human-implementation`，未进 `origin/main`）。它把 `api/internal/modules/digitalhuman/candidates.go` 的门禁由
   `!supports(digital_human) && !supports(agent)`（支持 dh **或** agent 即可）
   收紧为 `!supports(digital_human)`（必须显式支持 dh）。
3. 而 `api/internal/modules/skills/runtime_document.go` 的 `normalizeSkillCallers` 对**系统** Skill 的默认值是 `[aichat, agent]`（不含 digital_human），整个 catalog 里只有 `file-generator/SKILL.md` 声明了 `- digital_human` → 收紧后只剩它一个。
4. 时间差解释：`zgi-api:digitalhuman-delivery-20260913-r3`（9-13）早于该提交，所以此前人工验收仍是旧逻辑；本次为下线 Exa 重建 api 镜像后，该提交才真正生效。

与 Exa 下线的关系：被移除的 direct skill 只额外注入 1 条候选（Exa Web Search），候选数是 `N+1 → N`，不是本次现象的成因。

修复（按确认选择「让 digital_human 成为默认调用方」）：

- `api/internal/modules/skills/runtime_document.go`：系统 Skill 未声明 `supported_callers` 时的默认值改为 `[aichat, agent, digital_human]`；显式声明 callers 的 Skill 不受影响（仍以声明为准）。
- `api/internal/modules/skills/runtime_catalog_test.go`：原 `TestSystemSkillCallerDefaultsDoNotIncludeDigitalHuman`（锁定旧的严格默认）改为 `TestSystemSkillCallerDefaultsIncludeDigitalHuman`。
- `candidates.go` 的 `digital_human` 门禁保留：选择器只列「声明支持数字人」的 Skill，通用 Skill 现在通过默认值满足该条件。

验证：

- `go test ./internal/modules/skills/`：仅剩 2 个**改动前就存在**的失败（`TestScriptModelDoesNotReplayAndRejectsSkippedSequence`、`TestScriptExitCancelsPendingModel`，已在 clean tree 上复现），调用方默认值相关用例通过；`go test ./internal/modules/tools/... ./internal/modules/app/agents/... ./internal/capabilities/chatruntime/...` 中 `chatruntime/service`、`tools/handler` 等全部 ok（`chatruntime/repository` 的 2 个 branch 分页用例同样是改动前失败）。
- 真实接口：候选 **1 → 15**——`architecture-diagram-generator`、`content-summary`、`contract-field-extractor`、`decision-support`、`email-writing`、`file-generator`、`form-interaction`、`image-generator`、`multi-document-compare`、`ppt-slide-planner`、`prompt-professionalizer`、`response-tone-polisher`、`resume-screening`、`sensitive-redaction`、`ticket-routing`，全部 `available=true`。
- 真实接口 E2E：创建 → 绑定 **`content-summary`**（默认调用方 Skill，非 file-generator）→ 发布（active）→ 下架（disabled）→ 删除，全部通过；E2E 数据已清理（库中 0 条）。

仍不可选的（各自在 SKILL.md 里显式声明了 callers 且不含 `digital_human`）：`file-reader`（只声明 `aichat`，在 9-14 收紧之前同样不可选）。

补充（按确认追加）：给另外 5 个显式声明 callers 的 Skill 的 `supported_callers` 追加了 `- digital_human` —— `calculator`、`chart-generator`、`time`、`schedule-planner`、`work-report-generator`（它们原本都支持 `agent`，因此运行时门禁 `effectiveAgentSkillIDs` 早已允许数字人执行，只是选择器不显示）。追加后候选为 **20 个**（组织已启用的 21 个中仅 `file-reader` 仍不可选）。

补充验证：`go test ./internal/modules/skills/`（仍只剩 2 个既有失败）、`./internal/modules/tools/...`、`./internal/capabilities/chatruntime/service/`、`./internal/modules/app/agents/` 全部 ok；真实接口 E2E：创建 → 候选 20（含 5 个新开放的）→ 绑定 `calculator` + `time` → 发布（active）→ 下架（disabled）→ 删除，通过；E2E 数据已清零。

### 16.9 前端：任一 section 的加载/错误态被外部应用目录污染

现象：后端接口已返回 20 个候选（浏览器请求 200），但「添加 Skill」弹窗仍然显示「没有匹配且可绑定的资源。／重试」。

定位：`digital-human-resource-bindings.tsx` 把外部应用目录查询的错误并进了**所有** section 的错误态：

```tsx
loading={activeCandidateQuery.isLoading || integrationCatalogQuery.isLoading}
error={activeCandidateQuery.isError || integrationCatalogQuery.isError}
```

而 `useIntegrationCatalog(true, 'shared')` 是常开查询；本栈 `EXTERNAL_INTEGRATIONS_ENABLED=false`（`docker/docker-compose.yaml:207`），`RegisterIntegrationRoutes` 因此不会注册（`api/routes/v1/routes.go:88`），`GET /console/api/integrations/catalog` 恒为 404（api 日志已确认）→ Skill 弹窗永远走「错误空态」。该写法自 `0d2bcdba9`（2026-08-23）起就存在，只在关闭外部集成时暴露。

修复：加载/错误态按 section 收敛，目录查询只影响 external apps 弹窗：

```tsx
loading={activeCandidateQuery.isLoading ||
  (activeDefinition.key === 'integrations' && integrationCatalogQuery.isLoading)}
error={activeCandidateQuery.isError ||
  (activeDefinition.key === 'integrations' && integrationCatalogQuery.isError)}
```

回归防护：`web/scripts/test-digital-human-console.mjs` 增加两条断言，锁定「section 只反映支撑该 section 的查询状态」。

验证：`tsc --noEmit`、ESLint 干净；`test:digital-human-console`（含新断言）、`test:digital-human-create-flow`、`test:digital-human-chat-route-sync`、`test:digital-human-api-docs`、`test:route-access`、`test:console-navigation-routes` 全部通过；web 镜像已重建并滚动重启（容器运行镜像 ID 与新构建一致），配置页 200。

遗留（非阻塞）：本栈关闭了外部集成，external apps 弹窗仍会显示目录加载失败——这是真实状态。若要在本机联调外部应用，需要把 `EXTERNAL_INTEGRATIONS_ENABLED` 置为 `true` 并配置集成凭据。

### 16.10 创建时由用户选择 Skill；模板卡片 hover 用按钮顶替 tag 行

需求：①从模板创建 / 自定义创建时都向用户开放 Skill 选择，模板只作预选；②模板卡片原本两个按钮常驻占用底部空间（hover 才显示），改成 hover 时顶替 tag 区域，tag 区域保持恒定高度。

① Skill 选择：

- 后端新增**创建前**候选接口 `GET /console/api/digital-human-skill-candidates`（`candidates.go` 的 `ListSkillCandidates` → 复用 `skillCandidates`，与草稿编辑器同一套暴露/调用方门禁；`run_handler.go` 注册为 id 组之外的兄弟路由；权限映射为 `digital_human.create`，因为该列表只服务于创建）。新增单测 `TestRuntimeServiceListsSkillCandidatesBeforeCreation`。
- 前端：`digital-human.service.ts#listSkillCandidates`、`useDigitalHumanSkillCandidates`（query key `DIGITAL_HUMAN_KEYS.skillCandidates`）、payload 新增 `templateDefaultSkillIds` 与 `selectCandidateBindings`（纯函数，选择顺序按候选顺序归一）。
- 表单：只读 Skill 列表改为可选列表（`aria-pressed` 切换、显示已选数量、加载/失败/空态与重试、可选提示）。选择状态用 `null = 未触碰` 表示，模板默认值随候选异步到达后填充，一旦用户手动调整就不再被覆盖；自定义入口默认不预选。选择变化计入 dirty，会触发放弃确认。
- 提交：`bindSelectedSkills` 只绑定所选且在候选中的 Skill（不再调用按 id 的候选接口），一个都没选时不调用绑定接口；无论是否绑定都会发布第 1 版（已验证空选择也能发布）。

② 卡片：tag 行与操作按钮放进同一个 `relative mt-3 h-8` 槽位，tag 层默认可见、`sm:group-hover/focus-within` 时淡出，按钮层绝对定位于同槽位并在 hover/键盘聚焦时淡入——卡片高度恒定，按钮不再占用额外空间。小屏没有 hover，槽位直接显示按钮（tag 仍可在「详情」抽屉查看）。每个 `transition-*` 都配了 `motion-reduce:transition-none`（契约测试强制计数一致）。

验证：

- `go vet`、`go test ./internal/modules/digitalhuman/ -run TestRuntimeServiceLists`、`./internal/modules/skills/`、`./internal/modules/tools/...`、`./internal/capabilities/chatruntime/service/`、`./internal/modules/app/agents/` 全部通过（DH 包其余失败仍是 CGO/sqlite 既有环境限制）。
- 前端 `tsc --noEmit`、ESLint 干净；`test:digital-human-create-flow` 新增断言覆盖纯函数选择语义、表单可选交互与卡片单槽位结构；6 个 DH/路由/导航契约套件全绿。
- 真实接口 E2E：新接口返回 **20** 个创建前候选（与草稿编辑器一致，仍排除 `file-reader`）；创建后只绑定所选 `calculator` + `time`，发布版本经数据库核对**仅含这两条**；空选择创建同样发布成功（`skill-less creation published`）。E2E 数据已清零。
- api 与 web 镜像均已重建并滚动重启（容器镜像 ID 与新构建一致），`/console/digital-humans/create` 与 `?template=fileGeneration` 均 200，产物中可见新文案与新接口引用。

### 16.11 创建页的 Skill 展示/选择与配置页对齐（抽公共组件）

问题：创建页的 Skill 选择是自制的行式列表（无图标、无本地化标签、无搜索/来源筛选），与配置页弹窗的卡片网格不一致。

做法：把配置页的 Skill 呈现、匹配与"添加"弹窗抽成公共模块 `web/src/components/digital-human/digital-human-skill-picker.tsx`：

- `getDigitalHumanSkillDisplay` / `digitalHumanSkillSource` / `digitalHumanSkillToMetadata`（本地化标签、描述、图标、when_to_use，来自 AIChat skill display 元数据）
- `filterDigitalHumanSkills`（搜索 + 来源筛选，两处共用同一份匹配逻辑）
- `DigitalHumanSkillSourceSelect`（全部 / 系统 / 自定义）
- `DigitalHumanSkillGrid`（弹窗内卡片网格：图标 + 本地化标题 + 描述 + 选中圆圈，`aria-pressed` 切换，不可用则禁用）
- `DigitalHumanSkillBindingCard`（**配置页外层的已绑定卡片**：`ResourceBindingCard` + `AIChatSkillIcon` + 删除按钮）
- `DigitalHumanSkillDialogBody`（弹窗主体：骨架 / 失败重试 / 空态 / 无匹配 / 网格）
- `DigitalHumanSkillPickerDialog`（完整"添加 Skill"弹窗：`ResourceSelectionDialog` 外壳 + 标题/描述/已选计数 + 搜索 + 来源筛选 + 取消/保存）

对齐方式（按要求修正为"显示用配置外层卡片、添加用同一个弹窗"）：

- 配置页：`BindingCard` 在 `type === 'skill'` 时直接委托 `DigitalHumanSkillBindingCard`；弹窗主体在 `section === 'skills'` 时直接委托 `DigitalHumanSkillDialogBody`（其余 section 逻辑不变，本地 `getDigitalHumanSkillDisplay`/`skillSource`/内联网格/局部 Select 全部删除）。
- 创建页表单：Skill 区变成配置页同款 section 卡片（`Puzzle` 图标 + `bindings.sections.skills.title` + `bindings.selectedCount` 徽标 + `bindings.sections.skills.add` 的 `+` 按钮），正文在"已选则用 `DigitalHumanSkillBindingCard` 列表（带删除）、未选则用同款虚线添加块"之间切换；`+` / 添加块打开 `DigitalHumanSkillPickerDialog`，弹窗内是草稿选择，点"保存"才提交给创建流程（与配置页先选后存的交互一致）。

清理：删除只服务创建页旧列表的 i18n key（`createFlow.form.templateSkills` / `skillsTitle` / `skillsSelected` / `skillsEmpty` / `skillsUnavailable`），改为复用配置页既有 key，两个语言包同步；创建页只保留上下文特有文案（`skillsHint` / `skillsLoadFailed` / `skillsLoading` / `skillsOptional`）。

回归防护：`test-digital-human-create-flow.mjs` 断言创建页用公共已绑定卡片 + 公共弹窗（含 `pendingSkillIds` 草稿与 `saveSkillSelection` 提交）、`+` 用 `bindings.sections.skills.add`，并断言公共卡片是 `ResourceBindingCard` + `Trash2`、公共弹窗复用 `bindings.sections.skills.pickerTitle` 与 `bindings.skills.save`；`test-digital-human-console.mjs` 断言配置页渲染公共已绑定卡片 / 弹窗主体 / 来源筛选，且本地不得再定义 `getDigitalHumanSkillDisplay` 或 `skillSource`（对齐关系双向锁定）。

验证：`tsc --noEmit`、ESLint 干净；6 个前端契约套件全绿；web 镜像重建部署（容器镜像 ID 与新构建一致，创建页 200，产物含新组件引用）；真实接口复检：创建前候选 20 = 草稿候选 20，绑定 `schedule-planner` → 发布 → 下架 → 删除全部通过，数据已清理。
