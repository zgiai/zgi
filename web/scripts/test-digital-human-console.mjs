import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { URL } from 'node:url';

const list = await readFile(
  new URL('../src/components/digital-human/digital-human-list.tsx', import.meta.url),
  'utf8'
);
const configuration = await readFile(
  new URL('../src/components/digital-human/digital-human-configuration.tsx', import.meta.url),
  'utf8'
);
const configurationHub = await readFile(
  new URL('../src/components/digital-human/digital-human-configuration-hub.tsx', import.meta.url),
  'utf8'
);
const sidebar = await readFile(
  new URL('../src/components/digital-human/digital-human-sidebar.tsx', import.meta.url),
  'utf8'
);
const header = await readFile(
  new URL('../src/components/digital-human/digital-human-header.tsx', import.meta.url),
  'utf8'
);
const basicInfoDialog = await readFile(
  new URL('../src/components/digital-human/digital-human-basic-info-dialog.tsx', import.meta.url),
  'utf8'
);
const resourceBindings = await readFile(
  new URL('../src/components/digital-human/digital-human-resource-bindings.tsx', import.meta.url),
  'utf8'
);
const service = await readFile(
  new URL('../src/services/digital-human.service.ts', import.meta.url),
  'utf8'
);
const digitalHumanTypes = await readFile(
  new URL('../src/services/types/digital-human.ts', import.meta.url),
  'utf8'
);
const digitalHumanZhHans = await readFile(
  new URL('../src/i18n/modules/digitalHumans/zh-Hans.ts', import.meta.url),
  'utf8'
);
const digitalHumanEnUS = await readFile(
  new URL('../src/i18n/modules/digitalHumans/en-US.ts', import.meta.url),
  'utf8'
);
const hooks = await readFile(
  new URL('../src/hooks/digital-human/use-digital-humans.ts', import.meta.url),
  'utf8'
);
const runs = await readFile(
  new URL('../src/components/digital-human/digital-human-runs.tsx', import.meta.url),
  'utf8'
);
const runDetail = await readFile(
  new URL('../src/components/digital-human/digital-human-run-detail-drawer.tsx', import.meta.url),
  'utf8'
);
const agentRunDetail = await readFile(
  new URL(
    '../src/app/console/agents/[agentId]/logs/_components/agent-runtime-log-detail-drawer.tsx',
    import.meta.url
  ),
  'utf8'
);
const chat = await readFile(
  new URL('../src/components/digital-human/digital-human-chat.tsx', import.meta.url),
  'utf8'
);
const runtimeTransport = await readFile(
  new URL('../src/components/chat/transports/digital-human-runtime-transport.ts', import.meta.url),
  'utf8'
);
const detailPage = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/page.tsx', import.meta.url),
  'utf8'
);
const dashboard = await readFile(
  new URL('../src/components/digital-human/digital-human-dashboard.tsx', import.meta.url),
  'utf8'
);
const layout = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/layout.tsx', import.meta.url),
  'utf8'
);
const settingsPage = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/settings/page.tsx', import.meta.url),
  'utf8'
);
const preferencesPage = await readFile(
  new URL(
    '../src/app/console/digital-humans/[digitalHumanId]/preferences/page.tsx',
    import.meta.url
  ),
  'utf8'
);
const capabilitiesPage = await readFile(
  new URL(
    '../src/app/console/digital-humans/[digitalHumanId]/capabilities/page.tsx',
    import.meta.url
  ),
  'utf8'
);
const configurationPage = await readFile(
  new URL(
    '../src/app/console/digital-humans/[digitalHumanId]/configuration/page.tsx',
    import.meta.url
  ),
  'utf8'
);
const memory = await readFile(
  new URL('../src/components/digital-human/digital-human-memory.tsx', import.meta.url),
  'utf8'
);
const channels = await readFile(
  new URL('../src/components/digital-human/digital-human-channels.tsx', import.meta.url),
  'utf8'
);
const channelHooks = await readFile(
  new URL('../src/hooks/digital-human/use-digital-human-channels.ts', import.meta.url),
  'utf8'
);
const channelProviders = await readFile(
  new URL('../src/components/digital-human/digital-human-channel-providers.ts', import.meta.url),
  'utf8'
);
const feishuChannelProvider = await readFile(
  new URL('../src/components/digital-human/channel-providers/feishu.tsx', import.meta.url),
  'utf8'
);
const dingTalkChannelProvider = await readFile(
  new URL('../src/components/digital-human/channel-providers/dingtalk.tsx', import.meta.url),
  'utf8'
);
const channelPage = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/channels/page.tsx', import.meta.url),
  'utf8'
);
const taskPage = await readFile(
  new URL('../src/app/console/digital-humans/[digitalHumanId]/tasks/page.tsx', import.meta.url),
  'utf8'
);
const tasks = await readFile(
  new URL('../src/components/digital-human/digital-human-tasks.tsx', import.meta.url),
  'utf8'
);
const taskHooks = await readFile(
  new URL('../src/hooks/digital-human/use-digital-human-tasks.ts', import.meta.url),
  'utf8'
);

for (const [name, source] of [
  ['list', list],
  ['configuration', configuration],
]) {
  assert.match(
    source,
    /useAvailableModels\(\{ use_case: 'agent-runtime' \}\)/,
    `${name} must use agent-runtime candidates`
  );
  assert.doesNotMatch(
    source,
    /availableModels\.models\.filter\([^)]*callable/,
    `${name} must not filter the compact available-model response by its omitted callable field`
  );
}

assert.doesNotMatch(
  service,
  /[`'"/]agents(?:[`'"/]|$)/,
  'DigitalHuman service must not call Agent endpoints'
);
assert.match(
  memory,
  /useDigitalHumanMemories[\s\S]*memory_class[\s\S]*scope_type[\s\S]*scope_id[\s\S]*status[\s\S]*source/,
  'DigitalHuman memory must expose class, scope type, scope ID, status, and source filters'
);
assert.match(
  memory,
  /evidence[\s\S]*last_operation_id[\s\S]*undoMemory/,
  'DigitalHuman memory must expose evidence, provenance, and undo'
);
assert.match(
  memory,
  /ConfirmDialog[\s\S]*invalidateTitle/,
  'DigitalHuman memory invalidation must require confirmation'
);
assert.doesNotMatch(
  memory,
  /MEMORY_KEYS\.me|\/console\/api\/agents\//,
  'DigitalHuman memory must not use account memory or Agent endpoints'
);

assert.match(
  service,
  /\/digital-humans\/\$\{id\}\/draft\/binding-candidates/,
  'DigitalHuman bindings must use the independent resource candidate endpoint'
);
assert.match(
  resourceBindings,
  /useDigitalHumanBindingCandidates/,
  'DigitalHuman configuration must use searchable resource candidates'
);
assert.doesNotMatch(
  resourceBindings,
  /JSON\.parse\(metadata|setResourceId|setRevision/,
  'DigitalHuman configuration must not require raw IDs, revisions, or metadata JSON'
);
assert.match(
  header,
  /configuration\.preflight\.title/,
  'DigitalHuman publication must show a preflight checklist'
);
assert.match(
  header,
  /role=\{permissions\.hasPermission\('digital_human\.update'\)[\s\S]*setBasicInfoOpen\(true\)/,
  'DigitalHuman header identity block must open display editing like the Agent header'
);
assert.match(
  basicInfoDialog,
  /IconInput[\s\S]*showAvatarPresets/,
  'DigitalHuman display editing must reuse the Agent text, preset, and uploaded image icon flow'
);
assert.match(basicInfoDialog, /icon_type:[\s\S]*icon:/);
assert.match(
  list,
  /IconPreview[\s\S]*iconType=\{item\.icon_type/,
  'DigitalHuman list cards must render the same persisted icon representation as Agent cards'
);
assert.match(
  configuration,
  /PersonaTemplateActions[\s\S]*PersonaTemplateLibrary[\s\S]*PersonaTemplateEditor/,
  'DigitalHuman settings must provide editable persona-and-rule templates'
);
assert.doesNotMatch(
  configuration,
  /draft\.responsibilities|role_template_source|configuration\.preflight\.workProfile/,
  'DigitalHuman configuration must not persist a separate required work profile or role provenance'
);
assert.match(
  configuration,
  /identity_name:[\s\S]*goals:[\s\S]*rules:/,
  'DigitalHuman settings must version the conversation name, optional goals, and rules together'
);
assert.doesNotMatch(
  configuration,
  /snapshot\.definition|fields\.description/,
  'Workspace display metadata must not be saved as part of the runtime configuration draft'
);
assert.doesNotMatch(
  configuration,
  /function RolesCard|configuration\.preflight\.roles/,
  'DigitalHuman configuration must not present role assignment as the primary required workflow'
);
assert.match(
  detailPage,
  /DigitalHumanDashboard[\s\S]*id=\{digitalHumanId\}/,
  'DigitalHuman detail root must render the operational workspace dashboard'
);
assert.match(
  list,
  /item\.published_version_id[\s\S]*digital-humans\/\$\{item\.id\}[\s\S]*digital-humans\/\$\{item\.id\}\/configuration[\s\S]*router\.push\(`\/console\/digital-humans\/\$\{id\}\/configuration`\)/,
  'Unpublished DigitalHumans must enter initial configuration instead of the operational workspace'
);
assert.match(
  layout,
  /useDigitalHuman\(id\)[\s\S]*isInitialSetup[\s\S]*router\.replace\(configurationHref\)[\s\S]*routeGuardPending[\s\S]*initialSetup\.redirecting/,
  'Unpublished DigitalHuman routes must redirect to configuration without rendering child pages first'
);
assert.match(
  sidebar,
  /isInitialSetup[\s\S]*restrictToConfiguration[\s\S]*navItems = restrictToConfiguration \? \[configurationItem\] : publishedNavItems/,
  'Unpublished DigitalHumans must hide operational sidebar destinations'
);
assert.match(
  configuration,
  /isInitialSetup[\s\S]*initialSetupChecks[\s\S]*initialSetup\.title[\s\S]*initialSetup\.progress/,
  'Initial configuration must expose required-field progress until the first publish'
);
assert.match(
  configuration,
  /initialSetupChecks = \[[\s\S]*fields\.identityName[\s\S]*fields\.persona[\s\S]*\];/,
  'Initial configuration must require only the conversation name and persona'
);
assert.doesNotMatch(
  header,
  /configuration\.preflight\.rules/,
  'Publishing must not block on optional operating rules'
);
assert.match(
  header,
  /detail\?\.published \? \([\s\S]*DigitalHumanVersionPopover[\s\S]*detail\?\.published && permissions\.hasPermission\('digital_human\.run'\)/,
  'Version management and test chat must remain hidden until the first publish'
);
assert.doesNotMatch(
  detailPage,
  /redirect\(/,
  'DigitalHuman detail root must remain available as the operational workspace route'
);
assert.match(
  dashboard,
  /useDigitalHuman\(id\)[\s\S]*useDigitalHumanTasks\(id, currentLocalDayRunWindow\(\)\)[\s\S]*useDigitalHumanRuns\(id\)[\s\S]*useDigitalHumanChannels\(id, true\)/,
  'DigitalHuman dashboard must derive identity, tasks, runs, and connection state from live data'
);
assert.match(
  dashboard,
  /successful_run_count[\s\S]*dashboard\.completedToday/,
  'DigitalHuman dashboard must count successful task executions today rather than terminal task records'
);
assert.match(
  dashboard,
  /getAgentTextIconDisplay\([\s\S]*icon=\{textIcon\.text\}[\s\S]*iconBackground=\{textIcon\.background\}[\s\S]*definition\.icon_url \|\| definition\.avatar/,
  'DigitalHuman dashboard must render the same persisted text and image icon representation as Agent surfaces'
);
assert.match(
  dashboard,
  /tasks\?task=\$\{task\.id\}/,
  'DigitalHuman dashboard task rows must deep-link to the selected task detail'
);
assert.match(
  dashboard,
  /refreshDashboard[\s\S]*detail\.refetch\(\)[\s\S]*tasks\.refetch\(\)[\s\S]*runs\.refetch\(\)[\s\S]*channels\.events\.refetch\(\)/,
  'DigitalHuman dashboard must provide one manual refresh action for all visible status sources'
);
assert.match(
  dashboard,
  /DashboardLiveFeed[\s\S]*prefers-reduced-motion[\s\S]*dashboard\.live\.title/,
  'DigitalHuman dashboard must expose a reduced-motion-aware live activity surface'
);
assert.match(
  dashboard,
  /runs\.filter\(item => activeRunStatuses\.has\(item\.status\)\)/,
  'DigitalHuman live activity must only include currently active runs'
);
assert.match(
  dashboard,
  /!\['completed', 'canceled'\]\.includes\(task\.work_status\)/,
  'DigitalHuman workspace task board must hide terminal tasks by default'
);
assert.match(
  dashboard,
  /attentionPageSize = 3[\s\S]*buildDashboardAttentionItems[\s\S]*AttentionPagination/,
  'DigitalHuman attention items must share one prioritized, paginated queue'
);
assert.match(
  dashboard,
  /ignoreAttention[\s\S]*writeIgnoredAttentionKeys[\s\S]*restoreIgnoredAttention/,
  'DigitalHuman attention items must support reversible dashboard dismissal'
);
assert.match(
  service,
  /dismissChannelDelivery[\s\S]*\/deliveries\/\$\{deliveryId\}\/dismiss/,
  'DigitalHuman operators must be able to close a historical failed delivery without resending it'
);
assert.match(
  channels,
  /channels\.deliveries\.dismissDialog/,
  'DigitalHuman channel activity must confirm source-level delivery dismissal'
);
assert.match(
  tasks,
  /useState<StatusFilter>\('open'\)[\s\S]*openStatuses/,
  'DigitalHuman task management must default to unfinished work while preserving an all-status filter'
);
assert.match(
  hooks,
  /return active \? 1_500 : 10_000/,
  'DigitalHuman runs must refresh quickly while active and continue low-frequency idle polling'
);
assert.match(
  layout,
  /tabs\.configuration[\s\S]*tabs\.channels[\s\S]*tabs\.runs/,
  'DigitalHuman detail navigation must expose one configuration entry, channels, and logs'
);
assert.doesNotMatch(
  `${layout}\n${sidebar}`,
  /tabs\.overview/,
  'DigitalHuman detail navigation must not expose a redundant overview entry'
);
assert.match(
  layout,
  /DigitalHumanSidebar[\s\S]*DigitalHumanHeader/,
  'DigitalHuman detail must use an Agent-style resource sidebar and compact editor header'
);
assert.match(
  sidebar,
  /ResourceSidebar[\s\S]*usePersistentSidebarCollapse\('digitalHuman'[\s\S]*tabs\.configuration[\s\S]*tabs\.channels[\s\S]*tabs\.runs/,
  'DigitalHuman detail sidebar must expose configuration as one primary workbench entry'
);
assert.doesNotMatch(
  sidebar,
  /configurationHub\.sections\.|children:/,
  'DigitalHuman primary sidebar must leave configuration sections to the workbench secondary navigation'
);
assert.doesNotMatch(
  layout,
  /tabs\.(settings|preferences|capabilities)/,
  'DigitalHuman primary navigation must not split one draft across three configuration tabs'
);
assert.doesNotMatch(
  layout,
  /label: t\('tabs\.chat'\)/,
  'DigitalHuman test chat must not occupy a primary navigation tab'
);
assert.match(
  layout,
  /test-chat[\s\S]*SheetContent[\s\S]*DigitalHumanChat[\s\S]*controlsPortalId/,
  'DigitalHuman test chat must open from the header in a responsive Sheet'
);
assert.doesNotMatch(
  `${layout}\n${list}`,
  /digital-humans\/\$\{[^}]+\}\/chat|list\.open|list\.chat/,
  'DigitalHuman detail and list surfaces must not expose a standalone chat page'
);
assert.match(
  list,
  /grid-cols-\[repeat\(auto-fill,minmax\(15rem,1fr\)\)\][\s\S]*DigitalHumanCard[\s\S]*function DigitalHumanCard[\s\S]*<Link[\s\S]*className="block h-full/,
  'DigitalHuman list cards must follow the Agent responsive clickable-card layout'
);
assert.match(
  configurationHub,
  /legacySectionPanels[\s\S]*DigitalHumanConfigurationEditor/,
  'DigitalHuman configuration must keep old deep links while rendering one unified editor'
);
assert.doesNotMatch(
  configurationHub,
  /ResourceSidebar|aria-current|configurationHub\.navigation|configurationHub\.title|configurationHub\.description|router\.push/,
  'DigitalHuman configuration workbench must not duplicate the page header or use top-level tabs'
);
assert.match(
  configuration,
  /function ConfigurationEditor[\s\S]*configuration-identity[\s\S]*configuration-behavior[\s\S]*configuration-capabilities/,
  'DigitalHuman configuration must keep identity, response, and capability entry points in one editor'
);
assert.match(
  configuration,
  /lg:grid-cols-2[\s\S]*ConfigurationMenuItem[\s\S]*DigitalHumanResourceBindings/,
  'DigitalHuman configuration must split basic settings and resource bindings evenly'
);
assert.match(
  configuration,
  /configuration-model[\s\S]*ModelSelector[\s\S]*configuration-identity[\s\S]*fields\.identityName[\s\S]*fields\.persona[\s\S]*fields\.goals/,
  'DigitalHuman model and identity settings must remain directly editable in the left column'
);
assert.match(
  configuration,
  /ConfigurationMenuItem[\s\S]*setActivePanel\('response'\)[\s\S]*activePanel === 'response'/,
  'DigitalHuman channel response preferences must use the lower-frequency second-level editor'
);
assert.doesNotMatch(
  configuration,
  /<Card|CardContent|CardHeader|CardTitle|CardDescription/,
  'DigitalHuman unified configuration must not wrap each setting in a large independent card'
);
assert.match(
  configuration,
  /intermediateMessagesEnabled[\s\S]*acknowledgementEnabled[\s\S]*currentSnapshot[\s\S]*channel_response/,
  'DigitalHuman identity and channel response settings must share one draft snapshot'
);
assert.equal(
  [...configuration.matchAll(/useRegisterDigitalHumanDraftEditor\(id,/g)].length,
  1,
  'DigitalHuman unified configuration must register exactly one draft editor'
);
assert.match(
  configurationPage,
  /DigitalHumanConfigurationHub/,
  'DigitalHuman configuration must have one canonical route'
);
for (const [name, source, section] of [
  ['settings', settingsPage, 'identity'],
  ['preferences', preferencesPage, 'behavior'],
  ['capabilities', capabilitiesPage, 'capabilities'],
]) {
  assert.match(
    source,
    new RegExp(`redirect\\([\\s\\S]*configuration\\?section=${section}`),
    `Legacy DigitalHuman ${name} route must redirect to its configuration section`
  );
}
assert.match(
  configuration,
  /function RulesEditor[\s\S]*data-rule-input/,
  'operating rules must support line-by-line add and remove management'
);
assert.match(configuration, /settings\.addRule[\s\S]*settings\.removeRule/);
assert.match(
  configuration,
  /useDigitalHumanDraftPersistence[\s\S]*useRegisterDigitalHumanDraftEditor/,
  'DigitalHuman settings must register Agent-style automatic and manual draft persistence with the shared header'
);
assert.match(
  header,
  /useDigitalHumanDraftStore[\s\S]*(?:draftEditor|editor)\.saveNow/,
  'DigitalHuman header must expose draft save state and manual save'
);
assert.match(
  header,
  /useDigitalHumanVersions[\s\S]*DigitalHumanVersionPopover/,
  'DigitalHuman header must expose immutable version history'
);
assert.match(
  layout,
  /isConfigurationPage[\s\S]*<DigitalHumanHeader/,
  'DigitalHuman Agent-style draft header must only be mounted for the configuration workbench'
);
assert.match(
  service,
  /\/digital-humans\/\$\{id\}\/versions/,
  'DigitalHuman version history must use the scoped versions endpoint'
);
assert.doesNotMatch(
  configuration,
  /function DraftActionBar|sm:sticky sm:bottom-4/,
  'DigitalHuman settings must not use a floating draft and publish action bar'
);
assert.match(
  configuration,
  /ModelSelector[\s\S]*availabilityUseCase="agent-runtime"[\s\S]*preferredUseCase="agent"[\s\S]*features_tool_call/,
  'DigitalHuman settings must reuse the Agent model selector and only show Agent-capable tool models'
);
assert.match(
  resourceBindings,
  /grid-cols-1[\s\S]*SectionIcon[\s\S]*ResourceSelectionDialog[\s\S]*saveSelection/,
  'DigitalHuman capabilities must use direct vertical category cards and a batch resource picker'
);
assert.doesNotMatch(
  resourceBindings,
  /ResourceBindingSection|openSections/,
  'DigitalHuman resource category cards must not hide configuration behind collapsible sections'
);
assert.match(
  resourceBindings,
  /DatabaseBindingCards[\s\S]*groupDatabaseBindings/,
  'DigitalHuman database bindings must be grouped by database like Agent configuration'
);
assert.match(
  resourceBindings,
  /grid grid-cols-1 items-stretch gap-4/,
  'DigitalHuman capability categories must use a vertical card stack like Agent configuration'
);
assert.match(
  resourceBindings,
  /min-h-\[280px\] max-h-\[520px\][\s\S]*min-h-0 flex-1 overflow-y-auto/,
  'DigitalHuman capability cards must bound their height and scroll overflowing resources internally'
);
assert.doesNotMatch(
  resourceBindings,
  /<Card className="min-w-0 overflow-hidden"|<CardTitle>\{t\('bindings\.title'\)\}/,
  'DigitalHuman capability categories must not be wrapped in a redundant available-resources card'
);
assert.match(
  resourceBindings,
  /getAIChatSkillDisplayInfo[\s\S]*AIChatSkillIcon[\s\S]*skillSourceFilter/,
  'DigitalHuman Skills must reuse Agent display metadata, icons, and source filters'
);
assert.doesNotMatch(
  resourceBindings,
  /orderedCandidates|Number\(rightSelected\)\s*-\s*Number\(leftSelected\)/,
  'DigitalHuman resource dialogs must preserve candidate order while selections change'
);
assert.doesNotMatch(
  resourceBindings,
  /toast\.success\(t\('bindings\.saved'\)\)/,
  'DigitalHuman resource changes must not emit a success toast for every update'
);
assert.match(
  resourceBindings,
  /aria-pressed=\{Boolean\(selected\)\}[\s\S]*border-primary bg-primary\/5[\s\S]*<Check/,
  'DigitalHuman Skill selection must match the Agent card selection pattern'
);
assert.match(
  resourceBindings,
  /type DatabasePickerStep = 'databases' \| 'tables'[\s\S]*selectedDatabaseIds[\s\S]*openDatabaseTables[\s\S]*allowWriteForTable/,
  'DigitalHuman databases must use the Agent-style database, table, and write-permission flow'
);
assert.match(
  resourceBindings,
  /AgentWorkflowTypeIcon[\s\S]*AgentWorkflowTypeBadge[\s\S]*bindings\.workflows\.updatedAt/,
  'DigitalHuman workflows must expose Agent workflow type and release metadata'
);
assert.match(
  resourceBindings,
  /useIntegrationCatalog[\s\S]*allowed_action_ids[\s\S]*toggleIntegrationAction[\s\S]*allowedActions/,
  'DigitalHuman external apps must bind a connection and an explicit minimum action set'
);
assert.doesNotMatch(
  resourceBindings,
  /role="checkbox"/,
  'DigitalHuman resource cards must not nest interactive controls inside checkbox roles'
);
for (const section of ['skills', 'knowledge', 'databases', 'workflows', 'integrations']) {
  assert.match(
    resourceBindings,
    new RegExp(`bindings\\.sections\\.${section}`),
    `DigitalHuman capabilities must expose the ${section} resource section`
  );
}
assert.doesNotMatch(
  list,
  /fields\.persona|fields\.rules|fields\.model/,
  'DigitalHuman creation must defer detailed configuration until after creation'
);
for (const [name, source] of [
  ['list', list],
  ['configuration', configuration],
]) {
  assert.match(source, /DialogBody/, `${name} dialogs must use the shared padded body`);
}
for (const primitive of ['StickyDataTable', 'RunStatusBadge', 'DigitalHumanRunDetailDrawer']) {
  assert.match(runs, new RegExp(primitive), `DigitalHuman run history must use ${primitive}`);
}
assert.match(
  runDetail,
  /AgentRuntimeLogDetailDrawer[\s\S]*useDigitalHumanRunRuntimeLog/,
  'DigitalHuman run details must reuse the Agent per-step runtime log drawer'
);
for (const primitive of [
  'runtimeSteps',
  "type === 'model_call'",
  'runtimeInput',
  'runtimeOutput',
]) {
  assert.ok(
    agentRunDetail.includes(primitive),
    `The shared runtime log drawer must expose ${primitive}`
  );
}
assert.match(
  chat,
  /AIChatShell[\s\S]*createDigitalHumanRuntimeTransport[\s\S]*variant="embedded"[\s\S]*embeddedConversationMode="drawer"/,
  'DigitalHuman chat must use the shared embedded conversation Sheet UI'
);
assert.match(
  chat,
  /resolveConversationRouteSync[\s\S]*routeHandoffRef[\s\S]*isConversationTransitioning/,
  'DigitalHuman chat must coordinate URL handoffs and suppress stale conversation content'
);
assert.doesNotMatch(
  chat,
  /themeColor="violet"/,
  'DigitalHuman chat must use the shared conversation primary color'
);
assert.doesNotMatch(
  chat,
  /controller\.init\(initialConversationId\)|controller\.select\(conversationId\)/,
  'DigitalHuman chat must not race direct controller selection against route synchronization'
);
assert.match(
  runtimeTransport,
  /submitInvocation[\s\S]*recoverConversationStream/,
  'DigitalHuman chat turns must enter through durable runs and recover from ChatRuntime'
);
assert.match(
  runtimeTransport,
  /supportsRuntimeInput:\s*true/,
  'DigitalHuman console chat must reuse the shared running-input queue'
);
assert.doesNotMatch(
  `${chat}\n${runtimeTransport}`,
  /MEMORY_KEYS\.me|\/console\/api\/agents\//,
  'DigitalHuman chat must not use account memory or Agent endpoints'
);
assert.match(
  hooks,
  /digital_human\.revision\.conflict[\s\S]*invalidateDetail/,
  'DigitalHuman optimistic conflicts must refetch the server draft'
);
assert.match(runDetail, /useDigitalHumanRun/, 'Run details must load the latest durable run state');
assert.match(
  service,
  /\/digital-humans\/\$\{id\}\/runs\/\$\{runId\}\/runtime-log/,
  'DigitalHuman service must load the scoped ChatRuntime step projection'
);
assert.doesNotMatch(
  `${runs}\n${runDetail}`,
  /JSON\.stringify\(run\.(run_spec|execution_snapshot)/,
  'Run details must not dump raw execution snapshots into the console'
);
assert.match(
  service,
  /\/digital-humans\/\$\{id\}\/channels\/installations[\s\S]*\/digital-humans\/\$\{id\}\/channels\/events[\s\S]*\/digital-humans\/\$\{id\}\/channels\/deliveries/,
  'DigitalHuman service must expose installation and audit endpoints'
);
assert.doesNotMatch(
  service,
  /channels\/(bindings|discoveries)|createChannelBinding|deleteChannelBinding|listChannelBindings/,
  'Conversation routing must remain automatic and must not expose manual binding APIs'
);
assert.match(
  channelPage,
  /DigitalHumanFeatureGate permission="digital_human\.view"[\s\S]*DigitalHumanChannels/,
  'DigitalHuman channel page must use the feature and view-permission gate'
);
assert.doesNotMatch(
  channels,
  /BindingPanel|CreateBindingDialog|createBinding|deleteBinding|channels\.bindings/,
  'DigitalHuman primary UI must not expose conversation binding management'
);
assert.match(
  channels,
  /useDigitalHumanChannels\(id,\s*canViewActivity\)/,
  'DigitalHuman primary UI must load only installation and audit data'
);
assert.match(
  channelProviders,
  /driverId: 'feishu-open-api'[\s\S]*authMethodId: 'feishu_tenant_app'[\s\S]*credentialSource: 'organization'[\s\S]*connectionFilter:/,
  'DigitalHuman channels must define provider-owned credential contracts instead of accepting arbitrary connections'
);
assert.match(
  channelProviders,
  /id: 'dingtalk'[\s\S]*driverId: 'dingtalk-open-api'[\s\S]*authMethodId: 'organization_dingtalk_internal_app'[\s\S]*DingTalkCredentialFields/,
  'DigitalHuman channels must register DingTalk through the same provider-owned UI boundary'
);
assert.doesNotMatch(
  channels,
  /Feishu|feishu|app_id|app_secret|verification_token|encrypt_key/,
  'The generic DigitalHuman channel page must not contain Feishu-specific branches or credential fields'
);
assert.match(
  channels,
  /const wasOpen = useRef\(false\)[\s\S]*open && !wasOpen\.current[\s\S]*createInstallation\.mutateAsync[\s\S]*connection_id: selectedConnectionID[\s\S]*credentials: provider\.credentialUI\.serialize/,
  'DigitalHuman channel creation must reset the form and submit either an installation credential or a provider-filtered Connection Center reference'
);
assert.match(
  channels,
  /provider\?\.connectionFilter[\s\S]*credentials\.filter\(provider\.connectionFilter\)/,
  'DigitalHuman channel creation must delegate optional Connection Center filtering to the selected provider'
);
assert.match(
  channels,
  /<Select[\s\S]*<SelectTrigger[\s\S]*<SelectValue[\s\S]*<SelectContent[\s\S]*<SelectItem/,
  'DigitalHuman Connection Center selection must use the shared system Select component'
);
assert.doesNotMatch(
  channels,
  /<select\b/,
  'DigitalHuman Connection Center selection must not fall back to a native select'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /CredentialLocationGuide[\s\S]*guide\.appCredentials[\s\S]*guide\.eventSecurity[\s\S]*open\.feishu\.cn\/app/,
  'DigitalHuman Feishu credential forms must explain where users can obtain each required value'
);
assert.doesNotMatch(
  feishuChannelProvider,
  /function CredentialLocationGuide\(\)[\s\S]*?return \(\s*<details/,
  'The pre-save Feishu checklist must remain visible instead of hiding the highest-priority guidance'
);
assert.match(
  feishuChannelProvider,
  /guide\.afterSaveHint[\s\S]*FeishuInstallationSetupGuide[\s\S]*postSave\.steps/,
  'Feishu setup must separate pre-save credential preparation from post-save callback and publication steps'
);
assert.match(
  `${channels}\n${channelHooks}\n${feishuChannelProvider}`,
  /verificationResults[\s\S]*mutateAsync[\s\S]*VerificationStatus[\s\S]*failure\.permission_missing/,
  'Channel verification must render provider-specific actionable results instead of only a generic toast'
);
assert.match(
  `${channels}\n${channelHooks}\n${feishuChannelProvider}\n${service}\n${digitalHumanTypes}`,
  /startConnectionTest[\s\S]*connection-tests[\s\S]*getChannelConnectionTest[\s\S]*message_received[\s\S]*reply_delivered/,
  'Feishu connection testing must require backend-confirmed inbound and final-delivery evidence'
);
assert.match(
  `${channels}\n${feishuChannelProvider}\n${service}\n${digitalHumanTypes}\n${digitalHumanZhHans}`,
  /conversation_type[\s\S]*groupPrompt[\s\S]*group_mention[\s\S]*message_history[\s\S]*member_names[\s\S]*reply_delivery[\s\S]*im:message:readonly[\s\S]*im:chat\.members:read/,
  'Feishu group testing must expose an automatic capability matrix backed by one real group mention'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /FeishuGroupPermissionGuide[\s\S]*im:message\.group_msg[\s\S]*im:message:send_as_bot[\s\S]*im:message:readonly[\s\S]*im:chat\.members:read[\s\S]*数字人不会启动[\s\S]*最终回复无法发送[\s\S]*此前对话/,
  'Feishu group diagnostics must explain each permission code and its user-visible missing-permission behavior'
);
assert.doesNotMatch(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /message_observed_by_another_connection|另一个飞书连接观察到了同一测试文案/,
  'Feishu connection-test results must not use another digital human connection as evidence for the current connection'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /FeishuMessageRoundTripTest[\s\S]*test_message[\s\S]*eventChecklist\.eventPage[\s\S]*im\.message\.receive_v1/,
  'Feishu setup must guide a real direct-message test and distinguish Event Configuration from Callback Configuration'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /userConfirmedSent[\s\S]*confirmSent[\s\S]*callbackNotReceived[\s\S]*eventChecklistTitle/,
  'Feishu callback diagnostics must wait for the user to confirm that the real test message was sent'
);
assert.match(
  digitalHumanZhHans,
  /新增或修改地址后必须保存并重新发布应用/,
  'Feishu setup must explain that callback URL changes require republishing the app'
);
assert.match(
  digitalHumanZhHans,
  /飞书后台开通或审批完成后会直接生效，无需发布新版本/,
  'Feishu permission guidance must explain that permission-only changes do not require a new app version'
);
assert.match(
  digitalHumanZhHans,
  /权限生效后直接重新测试；只有修改可用范围或应用配置时才需要发布新版本/,
  'Feishu setup must distinguish permissions from versioned app configuration'
);
assert.doesNotMatch(
  digitalHumanZhHans,
  /开通后请重新发布应用|补齐权限并重新发布|权限后，必须发布新版本|im:message:send_as_bot，并发布新版本|im:message:readonly 和 im:message.group_msg，然后重新发布应用/,
  'Feishu permission guidance must not tell users to publish a new version for permission-only changes'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /permission_or_resource_denied[\s\S]*im:message:send_as_bot[\s\S]*停止自动重试/,
  'Feishu round-trip failures must expose confirmed send permission errors without implying continued retries'
);
assert.match(
  `${channels}\n${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /onConnectionTestChange[\s\S]*polled(?:Connection)?Test[\s\S]*role="alert"[\s\S]*连接测试已确认失败/,
  'Feishu connection-test terminal failures must become durable page state and render as a visible alert'
);
assert.match(
  `${feishuChannelProvider}\n${digitalHumanZhHans}`,
  /onStartConnectionTest\?\.\('group'\)[\s\S]*terminalFailure && onRestart[\s\S]*restartTest[\s\S]*重新测试/,
  'A terminal Feishu connection test must start a new test instead of refreshing an immutable expired result'
);
assert.match(
  dingTalkChannelProvider,
  /DINGTALK_CREDENTIAL_KEYS = \['app_key', 'app_secret'\][\s\S]*callbackTitle[\s\S]*open-dev\.dingtalk\.com/,
  'DigitalHuman DingTalk credential forms must require only app credentials and explain Stream setup'
);
assert.doesNotMatch(
  channels,
  /SelectContent container=\{dialogContainer\}/,
  'DigitalHuman channel selects must not portal into a transformed dialog container'
);
assert.doesNotMatch(
  `${channels}\n${service}`,
  /binding_count|used_by|remove_bindings|removeBindingsAndDelete/,
  'Automatic route cleanup must not leak binding mechanics into the product UI or client API'
);
assert.match(
  channels,
  /installation\.webhook_url[\s\S]*webhookDescription[\s\S]*webhookUnavailableHint/,
  'DigitalHuman channel setup must render the server-provided public webhook URL with actionable guidance'
);
assert.match(
  channels,
  /usesStream = installation\.provider === 'dingtalk'[\s\S]*usesStream \?[\s\S]*RadioTower[\s\S]*ChannelContextStatus/,
  'DingTalk installations must show Stream state and provider context capability without HTTP callback setup'
);
assert.match(
  channels,
  /status === 'not_supported'[\s\S]*channels\.installations\.context\.status\.\$\{status\}/,
  'Channel cards must distinguish unsupported history reads from temporary failures'
);
assert.match(
  digitalHumanZhHans,
  /not_supported: '当前平台只提供本次消息；需要更早内容时，请把原文复制到同一条 @ 消息中。'/,
  'The Chinese unsupported-history hint must explain the supported copy-and-paste recovery path'
);
assert.match(
  digitalHumanEnUS,
  /not_supported:[\s\S]*copy the source text into the same message that mentions the digital human/,
  'The English unsupported-history hint must explain the supported copy-and-paste recovery path'
);
assert.doesNotMatch(
  `${digitalHumanZhHans}\n${digitalHumanEnUS}`,
  /asks for quoted source text|引用上一条|回复上一条/,
  'Unsupported-history hints must not imply that provider quote metadata is available'
);
assert.doesNotMatch(
  channels,
  /external_tenant_id|external_bot_id|window\.location\.origin/,
  'DigitalHuman channel cards must not expose provider internals or guess callback URLs from the browser origin'
);
assert.match(
  channels,
  /configurationError[\s\S]*activityError[\s\S]*LoadFailure/,
  'DigitalHuman channels must distinguish load failures from empty states'
);
assert.match(
  channels,
  /event\.content_preview[\s\S]*event\.conversation_type[\s\S]*channels\.events\.receivedAt/,
  'DigitalHuman channel events must explain which external message triggered the activity'
);
assert.doesNotMatch(
  channels,
  /event\.(provider_event_id|provider_message_id|external_conversation_id|sender_external_id)/,
  'DigitalHuman channel activity must not expose provider technical identifiers'
);
assert.match(
  channels,
  /delivery\.status === 'failed' && canRetry/,
  'DigitalHuman channel UI must retry only known failed deliveries'
);
assert.doesNotMatch(
  `${channels}\n${channelHooks}\n${channelPage}`,
  /MEMORY_KEYS\.me|\/console\/api\/agents\/|wake_policy:\s*'ambient'/,
  'DigitalHuman channels must not use account memory, Agent endpoints, or ambient wake policies'
);

assert.match(
  taskPage,
  /DigitalHumanFeatureGate permission="digital_human\.view"[\s\S]*DigitalHumanTasks/,
  'DigitalHuman tasks must be available through the feature and view-permission gate'
);
assert.match(
  service,
  /listTasks\([\s\S]*createTask\([\s\S]*getTask\([\s\S]*updateTask\([\s\S]*applyTaskAction\([\s\S]*listTaskRuns\([\s\S]*listTaskActivities\(/,
  'DigitalHuman service must expose the complete task lifecycle and audit API'
);
assert.match(
  `${taskHooks}\n${service}`,
  /revision: number[\s\S]*applyTaskAction\(id, taskId, action, revision\)[\s\S]*expected_revision: expectedRevision/,
  'DigitalHuman task mutations must submit the current revision for optimistic concurrency'
);
assert.match(
  tasks,
  /ConfirmDialog[\s\S]*decline[\s\S]*cancel[\s\S]*archive/,
  'DigitalHuman task destructive actions must require explicit confirmation'
);
assert.match(
  tasks,
  /xl:grid-cols[\s\S]*TaskListCell label=/,
  'DigitalHuman tasks must provide mobile row labels and responsive table layout'
);
assert.match(
  tasks,
  /tasks\.columns\.nextRun[\s\S]*task\.next_run_at[\s\S]*tasks\.columns\.lastResult/,
  'DigitalHuman task rows must separate the next occurrence from the latest result'
);
assert.match(
  dashboard,
  /digitalHumanTaskDisplayStatus[\s\S]*tasks\.columns\.nextRun/,
  'DigitalHuman workbench must present recurring lifecycle state and next occurrence separately'
);
assert.doesNotMatch(
  `${channels}\n${taskHooks}\n${service}`,
  /task-policy|useChannelTaskPolicy|channel-task-policy-/,
  'External channel task creation must be available by default without a per-installation policy switch'
);
assert.match(
  feishuChannelProvider,
  /const onChangeRef = useRef\(onChange\)[\s\S]*onChangeRef\.current\?\.\(polledTest\)[\s\S]*\[polledTest, test\.id\]/,
  'Feishu connection-test polling must not resynchronize solely because a parent callback identity changed'
);

const sourceTabs = runs.match(/const sourceTabs:[\s\S]*?= \[([\s\S]*?)\n {2}\];/)?.[1] ?? '';
assert.ok(sourceTabs, 'DigitalHuman run source filters must remain statically inspectable');
assert.match(
  sourceTabs,
  /scheduled/,
  'DigitalHuman run filters must expose scheduled executions after task scheduling is delivered'
);
assert.doesNotMatch(
  sourceTabs,
  /email/,
  'DigitalHuman run filters must not expose email sources before email delivery exists'
);

console.log('DigitalHuman console contract checks passed.');
