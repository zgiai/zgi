import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();

function source(relativePath) {
  return readFileSync(path.join(root, relativePath), 'utf8');
}

function loadTypeScriptModule(relativePath) {
  const absolutePath = path.join(root, relativePath);
  const output = ts.transpileModule(readFileSync(absolutePath, 'utf8'), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      esModuleInterop: true,
    },
    fileName: absolutePath,
  }).outputText;
  const testModule = new Module(absolutePath);
  testModule.filename = absolutePath;
  testModule.paths = Module._nodeModulePaths(path.dirname(absolutePath));
  testModule._compile(output, absolutePath);
  return testModule.exports.default;
}

function leafPaths(value, prefix = '') {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix];
  return Object.entries(value).flatMap(([key, child]) =>
    leafPaths(child, prefix ? `${prefix}.${key}` : key)
  );
}

const english = loadTypeScriptModule('src/i18n/modules/integrations/en-US.ts');
const chinese = loadTypeScriptModule('src/i18n/modules/integrations/zh-Hans.ts');
const navigationEnglish = loadTypeScriptModule('src/i18n/modules/navigation/en-US.ts');
const navigationChinese = loadTypeScriptModule('src/i18n/modules/navigation/zh-Hans.ts');
assert.deepEqual(leafPaths(chinese).sort(), leafPaths(english).sort());
assert.equal(navigationEnglish.integrations, 'Connection Center');
assert.equal(navigationChinese.integrations, '连接中心');
assert.equal(english.connectionCenter.title, 'Connection Center');
assert.equal(chinese.connectionCenter.title, '连接中心');
assert.equal(english.connectionCenter.tabs.available, 'Available');
assert.equal(chinese.connectionCenter.tabs.available, '可连接');
assert.equal(english.connectionCenter.tabs.connected, 'Connected');
assert.equal(chinese.connectionCenter.tabs.connected, '已连接');
assert.equal(english.connectionCenter.quickConnect.connect, 'Connect');
assert.equal(chinese.connectionCenter.quickConnect.connect, '连接');
assert.equal(english.connectionCenter.connected.overview.label, 'Connection overview');
assert.equal(chinese.connectionCenter.connected.overview.label, '连接概览');
assert.equal(english.connectionCenter.connected.columns.usageTargets, 'Available tools');
assert.equal(chinese.connectionCenter.connected.columns.usageTargets, '可用工具');
assert.equal(english.connectionCenter.connected.enableAIChat, 'Choose in AIChat');
assert.equal(chinese.connectionCenter.connected.enableAIChat, '前往 AIChat 选择');
assert.equal(english.connectionCenter.connected.expandProvider, 'Expand {provider} connections');
assert.equal(chinese.connectionCenter.connected.expandProvider, '展开“{provider}”连接');
assert.equal(
  english.connectionCenter.connected.collapseProvider,
  'Collapse {provider} connections'
);
assert.equal(chinese.connectionCenter.connected.collapseProvider, '收起“{provider}”连接');
assert.equal(english.connectionCenter.connected.usageRules.manage, 'Manage usage rules');
assert.equal(chinese.connectionCenter.connected.usageRules.manage, '管理使用规则');
assert.equal(english.connectionCenter.connected.usageRules.configure, 'Configure usage rules');
assert.equal(chinese.connectionCenter.connected.usageRules.configure, '配置使用规则');
assert.equal(english.grants.title, 'Usage rules');
assert.equal(chinese.grants.title, '使用规则');
assert.equal(english.grants.scopeLabel, 'Who can use this connection?');
assert.equal(chinese.grants.scopeLabel, '谁可以使用这条连接？');
assert.equal(english.connectionDetail.scopes, 'Provider scopes');
assert.equal(chinese.connectionDetail.scopes, '服务商权限范围');
assert.equal(english.connectionHealth.scope, 'Provider scope check');
assert.equal(chinese.connectionHealth.scope, '服务商权限检查');
assert.equal(english.connectionCenter.advanced.policies, 'Action policies');
assert.equal(chinese.connectionCenter.advanced.policies, '操作策略');
assert.equal(chinese.metadata.providers.webSearch.name, '网页搜索');
assert.equal(chinese.metadata.actions.githubRepositoryList.name, '列出 GitHub 代码仓库');
assert.equal(chinese.enums.invokeFrom.agent, '智能体');
assert.equal(chinese.errors.integration_auth_invalid, '连接凭据无效');
assert.equal(chinese.oauth.clientConfig.setupGuide.toggle, '获取指南');
assert.equal(english.oauth.clientConfig.setupGuide.toggle, 'Credential guide');
assert.equal(english.providerDiagnostics.errorCode, 'Provider error code');
assert.equal(chinese.providerDiagnostics.errorCode, '服务商错误码');
assert.equal(english.providerDiagnostics.retryAfter, 'Retry after');
assert.equal(chinese.providerDiagnostics.retryAfter, '建议重试时间');

const chineseSource = source('src/i18n/modules/integrations/zh-Hans.ts');
assert.doesNotMatch(
  chineseSource,
  /\b(?:Agent|Credential|Action|Connector|Provider|Runtime)\b/,
  'zh-Hans external-app management copy contains untranslated business terminology'
);
assert.doesNotMatch(chineseSource, /API Key/, 'zh-Hans copy should use “API 密钥”');

const localeSources = [
  source('src/i18n/modules/integrations/en-US.ts'),
  source('src/i18n/modules/integrations/zh-Hans.ts'),
];
for (const localeSource of localeSources) {
  assert.doesNotMatch(localeSource, /�/);
  assert.match(localeSource, /metadata:/);
}

const deprecatedChineseAccessTerms =
  /连接授权|授权主体|添加授权|编辑授权|保存授权|删除授权|授权就绪/;
assert.doesNotMatch(
  JSON.stringify({
    connectionCenter: chinese.connectionCenter,
    connectionDetail: chinese.connectionDetail,
    grants: chinese.grants,
  }),
  deprecatedChineseAccessTerms
);
const deprecatedEnglishAccessTerms =
  /Connection grants?|Add grant|Edit grant|Save grant|authorization readiness|Actions & authorization/;
assert.doesNotMatch(
  JSON.stringify({
    connectionCenter: english.connectionCenter,
    connectionDetail: english.connectionDetail,
    grants: english.grants,
  }),
  deprecatedEnglishAccessTerms
);

const metadata = source('src/components/integrations/metadata-i18n.ts');
const integrationErrorI18n = source('src/services/integration-error-i18n.ts');
assert.match(metadata, /localizedMetadataValue/);
assert.match(metadata, /name_i18n/);
assert.match(metadata, /description_i18n/);
assert.match(metadata, /label_i18n/);
assert.match(metadata, /placeholder_i18n/);
assert.match(metadata, /useLocale/);
assert.match(metadata, /connection\.test/);
assert.match(metadata, /documentationURL/);
assert.match(metadata, /documentation_url_i18n/);
assert.match(metadata, /localizedLabelValue/);
assert.match(metadata, /category_labels_i18n/);
assert.match(metadata, /tag_labels_i18n/);
assert.match(metadata, /scope_labels_i18n/);
assert.match(metadata, /metadata\.categories\.unknown/);
assert.match(metadata, /metadata\.tags\.unknown/);
assert.match(metadata, /metadata\.scopes\.unknown/);
assert.match(metadata, /integrationErrorTranslationKey/);
assert.doesNotMatch(metadata, /const ERROR_TRANSLATIONS/);
assert.match(metadata, /if \(isChineseLocale\) return t\('common\.unknownExternalApp'\)/);
assert.match(metadata, /if \(isChineseLocale\) return authType\(auth\.type\)/);
assert.match(metadata, /if \(isChineseLocale\) return t\('dialog\.credentialField'\)/);
assert.match(integrationErrorI18n, /INTEGRATION_ERROR_TRANSLATION_KEYS/);
assert.match(integrationErrorI18n, /integrationErrorTranslationKeyFromError/);

const catalogComponent = source('src/components/integrations/provider-catalog.tsx');
const detailPage = source('src/app/dashboard/organization/integrations/[integrationId]/page.tsx');
const executionsPanel = source('src/components/integrations/executions-panel.tsx');
const providerDiagnostics = source('src/components/integrations/provider-diagnostics-details.tsx');
const connectionDetail = source('src/components/integrations/connection-detail-dialog.tsx');
const connectionPermissionSummary = source(
  'src/components/integrations/connection-permission-summary.tsx'
);
const connectionGrants = source('src/components/integrations/connection-grants-panel.tsx');
const connectionCenter = source('src/components/integrations/connection-center.tsx');
const connectionDialog = source('src/components/integrations/connection-dialog.tsx');
const connectionsPanel = source('src/components/integrations/connections-panel.tsx');
const capabilitiesSheet = source('src/components/integrations/provider-capabilities-sheet.tsx');
const capabilitiesInline = source('src/components/integrations/provider-capabilities-inline.tsx');
const capabilityView = source('src/components/integrations/provider-capability-view.ts');
const capabilityAvailability = source(
  'src/components/integrations/provider-capability-availability.tsx'
);
const integrationHooks = source('src/hooks/integrations/use-integrations.ts');
const integrationQueryKeys = source('src/hooks/query-keys.ts');
const authSetupGuide = source('src/components/integrations/auth-setup-guide.tsx');
const oauthClientConfig = source('src/components/integrations/oauth-client-config-dialog.tsx');
assert.doesNotMatch(catalogComponent, /safeIntegrationDisplayText\(item\.driver_id/);
assert.doesNotMatch(detailPage, /safeIntegrationDisplayText\(provider\.driver_id/);
assert.doesNotMatch(detailPage, /safeIntegrationDisplayText\(action\.id/);
assert.match(executionsPanel, /metadata\.actionNameByID/);
assert.match(executionsPanel, /ProviderDiagnosticsDetails/);
assert.match(providerDiagnostics, /safeOptionalIntegrationDisplayText\(providerErrorCode\)/);
assert.match(providerDiagnostics, /safeOptionalIntegrationDisplayText\(providerRequestId\)/);
assert.match(providerDiagnostics, /containsOpaqueUUID\(providerRequestId\)/);
assert.doesNotMatch(
  providerDiagnostics,
  /provider_(?:message|body|headers?|url)|arguments/i,
  'provider diagnostics UI must not accept unsafe upstream payload fields'
);
assert.doesNotMatch(connectionDetail, /item\.driver_id/);
assert.doesNotMatch(connectionGrants, /const actionCode/);
assert.doesNotMatch(connectionGrants, /<code[\s>]/);
assert.doesNotMatch(connectionGrants, /actionNames\.get\(actionId\)\s*\|\|\s*actionId/);
assert.match(catalogComponent, /metadata\.category\([^)]*,\s*(?:item|provider)\)/);
assert.match(catalogComponent, /metadata\.tag\(tag, item\)/);
assert.match(catalogComponent, /@container\/provider-catalog/);
assert.match(catalogComponent, /@container\/provider-card/);
assert.match(catalogComponent, /oauth\.clientConfig\.manageAction/);
assert.match(catalogComponent, /grid w-full grid-cols-\[minmax\(0,3fr\)_minmax\(0,2fr\)\] gap-1/);
assert.doesNotMatch(
  catalogComponent,
  /<span className="truncate">[\s\S]*connectionCenter\.quickConnect/
);
assert.match(catalogComponent, /IntegrationProviderCapabilitiesSheet/);
assert.doesNotMatch(catalogComponent, /audience=\{canManageShared/);
assert.match(catalogComponent, /hasExistingConnection/);
assert.doesNotMatch(catalogComponent, /title=\{t\('authMethodPicker\.otherMethods'\)\}/);
assert.match(capabilitiesSheet, /useCurrentAccountProviderCapabilityView/);
assert.doesNotMatch(capabilitiesSheet, /audience\?: 'account' \| 'organization'/);
assert.match(capabilitiesSheet, /hasExistingConnection/);
assert.match(capabilitiesInline, /useIntegrationActionPolicies/);
assert.match(capabilitiesInline, /useUpdateIntegrationActionPolicies/);
assert.match(capabilitiesInline, /approval_policy/);
assert.match(capabilitiesInline, /data_egress_allowed/);
assert.match(capabilitiesInline, /useCurrentAccountProviderCapabilityView/);
assert.match(
  capabilityView,
  /useIntegrationProviderCapabilities\(integrationId,\s*'account',\s*enabled\)/,
  'all user-facing capability views must include current-account connections even for administrators'
);
assert.match(capabilityView, /liveCapabilityByAction/);
assert.match(capabilityView, /authenticationMethods/);
assert.match(integrationQueryKeys, /capabilityLists/);
assert.match(
  integrationHooks,
  /useConnectionGrantMutation[\s\S]*INTEGRATION_KEYS\.capabilityLists\(\)/,
  'usage-rule changes must immediately invalidate capability availability'
);
assert.match(capabilityAvailability, /status_unavailable/);
assert.match(capabilityAvailability, /compatibleConnectionCount/);
assert.equal(chinese.capabilities.availability.needs_permission, '缺少使用权限');
assert.equal(english.capabilities.availability.needs_permission, 'Usage permission required');
assert.match(authSetupGuide, /guide\?\.steps/);
assert.match(authSetupGuide, /copy_callback_url/);
assert.match(authSetupGuide, /rel="noreferrer noopener"/);
assert.match(authSetupGuide, /metadata\.localizedText/);
assert.doesNotMatch(
  authSetupGuide,
  /integrationId\s*===|providerName\s*===/,
  'setup guide rendering must stay provider-driven'
);
assert.match(oauthClientConfig, /<AuthSetupGuide/);
assert.match(oauthClientConfig, /guide=\{auth\?\.setup_guide\}/);
assert.match(connectionDialog, /<AuthSetupGuide/);
assert.match(connectionDialog, /guide=\{selectedAuth\.setup_guide\}/);
assert.match(connectionDialog, /!oauthSelected && selectedAuth\?\.setup_guide/);
assert.match(
  authSetupGuide,
  /defaultOpen=\{guide\?\.expanded_by_default\}/,
  'setup guides may only default open when provider metadata explicitly requests it'
);
assert.doesNotMatch(authSetupGuide, /defaultOpen(?:=\{true\}|\s)/);
assert.doesNotMatch(oauthClientConfig, /defaultOpen/);
assert.doesNotMatch(connectionDialog, /defaultOpen/);
assert.doesNotMatch(connectionCenter, /IntegrationActionPoliciesPanel/);
assert.doesNotMatch(
  capabilitiesSheet,
  /connection(?:_|\.)id|provider\?\.connection_id/i,
  'capability discovery must not expose internal connection identifiers'
);
assert.match(detailPage, /redirect\(/);
assert.match(detailPage, /\/console\/integrations\?view=available&integration_id=/);
assert.match(connectionDetail, /IntegrationConnectionPermissionSummary/);
assert.match(connectionPermissionSummary, /summary\?\.adapted_capabilities/);
assert.match(connectionPermissionSummary, /const grantedScopeItems = grantedScopes \?\? \[\]/);
for (const permissionCollection of [
  'identity_permissions',
  'lifecycle_permissions',
  'provider_permissions',
  'unknown_permissions',
  'missing_permissions',
]) {
  assert.match(
    connectionPermissionSummary,
    new RegExp(`summary\\?\\.${permissionCollection} \\?\\? \\[\\]`),
    `${permissionCollection} must tolerate null responses during rolling upgrades`
  );
}
assert.match(connectionPermissionSummary, /metadata\.scope\(id, provider\)/);
assert.match(connectionPermissionSummary, /permissionSummary\.providerDetailsTitle/);
assert.match(connectionPermissionSummary, /permissionSummary\.providerNative/);
assert.match(connectionPermissionSummary, /\{permission\.id\}/);
assert.doesNotMatch(connectionCenter, /catalog\.platformAvailable|dialog\.platformCredential/);
assert.doesNotMatch(connectionDialog, /value="platform"/);
assert.doesNotMatch(connectionDialog, /dialog\.platformCredential|dialog\.platformUnavailable/);
assert.doesNotMatch(connectionsPanel, /['"`][^'"`]*(?:授权|Authorization)[^'"`]*['"`]/);
assert.match(connectionsPanel, /useQueries/);
assert.match(connectionsPanel, /summarizeUsageRules\(rules, validActionIDs\)/);
assert.match(
  connectionsPanel,
  /selectedConnectionIDs\.has\(connection\.id\)[\s\S]*availableConnectionIDs/
);
assert.match(connectionsPanel, /@container\/connections/);
assert.match(connectionsPanel, /@\[1120px\]\/connections:grid/);
assert.match(connectionsPanel, /function ConnectionOverviewStrip/);
assert.doesNotMatch(connectionsPanel, /function ConnectionJourney/);
assert.match(
  connectionsPanel,
  /aria-label=\{t\('connectionCenter\.connected\.overview\.label'\)\}/
);
assert.doesNotMatch(connectionsPanel, /md:right-\[calc\(-50%\+0\.875rem\)\]/);
assert.match(connectionsPanel, /initializedExpandedProvider = useRef\(false\)/);
assert.match(
  connectionsPanel,
  /if \(initializedExpandedProvider\.current \|\| groups\.length === 0\) return;[\s\S]*initializedExpandedProvider\.current = true;[\s\S]*setExpandedProviders\(new Set/
);
assert.equal((connectionsPanel.match(/<CollapsibleTrigger asChild>/g) ?? []).length, 1);
const addConnectionButtonIndex = connectionsPanel.indexOf('<AddConnectionButton');
const collapseTriggerIndex = connectionsPanel.indexOf('<CollapsibleTrigger asChild>');
assert.ok(addConnectionButtonIndex >= 0 && collapseTriggerIndex > addConnectionButtonIndex);
assert.match(
  connectionsPanel.slice(collapseTriggerIndex, collapseTriggerIndex + 1_500),
  /type="button"[\s\S]*isIcon[\s\S]*size-9[\s\S]*collapseProvider/
);
const primaryActionStart = connectionsPanel.indexOf('function ConnectionPrimaryAction');
const availableToolsStatusStart = connectionsPanel.indexOf(
  'function ConnectionAvailableToolsStatus',
  primaryActionStart
);
assert.ok(primaryActionStart >= 0 && availableToolsStatusStart > primaryActionStart);
assert.doesNotMatch(connectionsPanel, /function ConnectionUsageRulesAction/);
assert.doesNotMatch(
  connectionsPanel.slice(
    availableToolsStatusStart,
    connectionsPanel.indexOf('function AddConnectionButton')
  ),
  /<Button/
);
assert.match(
  connectionsPanel,
  /function ConnectionPrimaryAction[\s\S]*variant=\{needsSetup \? 'outline' : 'default'\}[\s\S]*onClick=\{needsSetup \? onContinueSetup : onConfigureTools\}[\s\S]*<ShieldCheck/
);
assert.equal((connectionsPanel.match(/<ConnectionPrimaryAction/g) ?? []).length, 1);
assert.match(connectionsPanel, /primaryActionNeedsSetup/);
assert.match(connectionsPanel, /connectionCenter\.connected\.usageRules/);
assert.match(connectionsPanel, /connectionCenter\.connected\.availableTools/);
assert.match(connectionsPanel, /connections\.actions\.disconnectAccount/);
assert.match(connectionsPanel, /disconnect\.description/);
assert.match(connectionsPanel, /confirmDisabled=\{deleteBlocked\}/);
assert.match(connectionsPanel, /closeOnConfirm=\{false\}/);
assert.match(connectionsPanel, /deleteMyMutation\.mutateAsync/);
assert.match(connectionsPanel, /deleteMutation\.mutateAsync/);
const policiesPanel = source('src/components/integrations/action-policies-panel.tsx');
const healthPanel = source('src/components/integrations/connection-health-panel.tsx');
assert.doesNotMatch(policiesPanel, /safeIntegrationDisplayText\(policy\.action_id/);
assert.match(policiesPanel, /metadata\.actionNameByID/);
assert.match(healthPanel, /metadata\.healthReason\(event\.reason_code\)/);
assert.match(healthPanel, /metadata\.scope\(scope, provider\)/);
assert.match(healthPanel, /ProviderDiagnosticsDetails/);
assert.doesNotMatch(healthPanel, /metadata\.error\(event\.reason_code\)/);

const dialogComponent = source('src/components/ui/dialog.tsx');
const languageSwitcher = source('src/components/common/language-switcher.tsx');
const i18nClientProvider = source('src/providers/i18n-client-provider.tsx');
assert.match(dialogComponent, /t\('close'\)/);
assert.doesNotMatch(dialogComponent, />Close</);
assert.match(languageSwitcher, /t\('switchLanguage'\)/);
assert.doesNotMatch(languageSwitcher, /aria-label="Switch language"/);
assert.match(i18nClientProvider, /document\.documentElement\.lang = resolvedLocale/);
assert.match(i18nClientProvider, /document\.documentElement\.lang = newLocale/);

const requiredMetadataConsumers = [
  'src/components/integrations/connection-center.tsx',
  'src/components/integrations/provider-catalog.tsx',
  'src/components/integrations/provider-capabilities-sheet.tsx',
  'src/components/integrations/provider-capabilities-inline.tsx',
  'src/components/integrations/action-policies-panel.tsx',
  'src/components/integrations/connection-dialog.tsx',
  'src/components/integrations/connection-detail-dialog.tsx',
  'src/components/integrations/connection-grants-panel.tsx',
  'src/components/integrations/connection-health-panel.tsx',
  'src/components/integrations/connections-panel.tsx',
  'src/components/integrations/executions-panel.tsx',
];
for (const relativePath of requiredMetadataConsumers) {
  assert.match(source(relativePath), /useIntegrationMetadata/);
}

const integrationsDirectorySources = [...requiredMetadataConsumers].map(source);
for (const componentSource of integrationsDirectorySources) {
  assert.doesNotMatch(componentSource, /formatDate\(/);
}

const consoleIntegrationsPage = source('src/app/console/integrations/page.tsx');
const consoleSidebar = source('src/components/console/console-sidebar.tsx');
assert.match(consoleIntegrationsPage, /IntegrationConnectionCenter/);
const integrationServiceSource = source('src/services/integration.service.ts');
const connectionCenterSource = source('src/components/integrations/connection-center.tsx');
const oauthRecoveryPanelSource = source('src/components/integrations/oauth-recovery-panel.tsx');
const providerCatalogSource = source('src/components/integrations/provider-catalog.tsx');
const connectionsPanelSource = source('src/components/integrations/connections-panel.tsx');
const connectedAppsSource = source('src/components/chat/variants/aichat/connected-apps-dialog.tsx');
assert.match(integrationServiceSource, /getAllConnections\(/);
assert.match(integrationServiceSource, /getAllMyConnections\(/);
assert.match(integrationServiceSource, /getOAuthRecovery\(/);
assert.match(integrationServiceSource, /acknowledgeOAuthRecovery\(/);
assert.match(connectionCenterSource, /canManageShared && oauthRecovery/);
assert.match(connectionCenterSource, /oauthRecovery\.unresolved_dead_letters > 0/);
assert.match(oauthRecoveryPanelSource, /provider_access_removed/);
assert.match(oauthRecoveryPanelSource, /token_confirmed_expired/);
assert.doesNotMatch(
  oauthRecoveryPanelSource,
  /operation\.auth_method_id|connection_id|client_secret|access_token|refresh_token/,
  'OAuth recovery UI must not render provider credentials or internal connection identifiers'
);
assert.doesNotMatch(
  oauthRecoveryPanelSource,
  />\s*\{operation\.operation_ref\}\s*</,
  'opaque recovery operation references must not be reader-visible'
);
for (const componentSource of [
  connectionCenterSource,
  providerCatalogSource,
  connectionsPanelSource,
]) {
  assert.match(componentSource, /useAllIntegrationConnections/);
  assert.match(componentSource, /useAllMyIntegrationConnections/);
}
assert.doesNotMatch(connectedAppsSource, /\/console\/settings#personal-connections/);
assert.match(connectedAppsSource, /\/console\/integrations\?view=connected/);
assert.doesNotMatch(consoleIntegrationsPage, /[\u3400-\u9fff]/);
assert.match(consoleSidebar, /title:\s*t\('integrations'\)/);
assert.match(consoleSidebar, /href:\s*'\/console\/integrations'/);
assert.doesNotMatch(consoleSidebar, /title:\s*['"](?:Connection Center|连接中心)['"]/);

const mutationHooks = source('src/hooks/integrations/use-integrations.ts');
assert.doesNotMatch(mutationHooks, /getErrorMessage/);
assert.match(mutationHooks, /integrationErrorTranslationKeyFromError/);
assert.doesNotMatch(mutationHooks, /const INTEGRATION_ERROR_TRANSLATIONS/);
assert.match(mutationHooks, /messages\.requestFailed/);

const skillSettings = source(
  'src/components/dashboard/organization/aichat-skill-settings-section.tsx'
);
assert.match(skillSettings, /useIntegrationMetadata\(\)/);
assert.match(skillSettings, /integrationMetadata\.providerName\(/);
assert.doesNotMatch(skillSettings, /skill\.availability\?\.reason/);
assert.doesNotMatch(skillSettings, /\?\?\s*integrationId/);

console.log('External app management i18n checks passed.');
