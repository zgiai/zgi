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
  /redirect\([\s\S]*\/configuration/,
  'DigitalHuman detail root must open the configuration workbench instead of a redundant overview'
);
assert.doesNotMatch(
  detailPage,
  /section=/,
  'DigitalHuman canonical configuration route must not require a tab query'
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
  /isConfigurationPage[\s\S]*\{isConfigurationPage \? \([\s\S]*<DigitalHumanHeader/,
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
  feishuChannelProvider,
  /CredentialLocationGuide[\s\S]*guide\.appCredentials[\s\S]*guide\.eventSecurity[\s\S]*open\.feishu\.cn\/app/,
  'DigitalHuman Feishu credential forms must explain where users can obtain each required value'
);
assert.match(
  dingTalkChannelProvider,
  /DINGTALK_CREDENTIAL_KEYS = \['app_key', 'app_secret'\][\s\S]*callbackTitle[\s\S]*open-dev\.dingtalk\.com/,
  'DigitalHuman DingTalk credential forms must require only app credentials and explain HTTP callback setup'
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

console.log('DigitalHuman console contract checks passed.');
