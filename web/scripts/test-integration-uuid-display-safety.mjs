import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const helperPath = path.join(root, 'src/components/integrations/display-utils.ts');
const helperSource = readFileSync(helperPath, 'utf8');
const output = ts.transpileModule(helperSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
  fileName: helperPath,
}).outputText;
const testModule = new Module(helperPath);
testModule.filename = helperPath;
testModule.paths = Module._nodeModulePaths(path.dirname(helperPath));
testModule._compile(output, helperPath);

const { containsOpaqueUUID, safeIntegrationDisplayText, safeOptionalIntegrationDisplayText } =
  testModule.exports;

const uuid = '123e4567-e89b-12d3-a456-426614174000';
const compactUuid = '123e4567e89b12d3a456426614174000';
assert.equal(containsOpaqueUUID(uuid), true);
assert.equal(containsOpaqueUUID(compactUuid), true);
assert.equal(containsOpaqueUUID('github.repository.list'), false);
assert.equal(safeIntegrationDisplayText(uuid, 'Unknown connection'), 'Unknown connection');
assert.equal(
  safeIntegrationDisplayText(`provider request ${uuid}`, 'Hidden reference'),
  'Hidden reference'
);
assert.equal(safeOptionalIntegrationDisplayText(compactUuid), null);
assert.equal(
  safeIntegrationDisplayText('github.repository.list', 'Unknown action'),
  'github.repository.list'
);

function source(relativePath) {
  return readFileSync(path.join(root, relativePath), 'utf8');
}

const connectionDetail = source('src/components/integrations/connection-detail-dialog.tsx');
assert.doesNotMatch(connectionDetail, /connectionDetail\.connectionId/);
assert.doesNotMatch(connectionDetail, /value=\{item\.id\}/);
assert.match(connectionDetail, /connectionDetail\.currentAccount/);
assert.match(connectionDetail, /safeIntegrationDisplayText\(item\?\.name/);

const executions = source('src/components/integrations/executions-panel.tsx');
assert.doesNotMatch(executions, /connectionNames\.get\([^)]*\)\s*\|\|\s*execution\.connection_id/);
assert.doesNotMatch(executions, /\{execution\.provider_request_id\}/);
assert.match(executions, /executions\.unknownConnection/);
assert.match(executions, /executions\.hiddenReference/);

const providerDiagnostics = source('src/components/integrations/provider-diagnostics-details.tsx');
assert.match(providerDiagnostics, /safeOptionalIntegrationDisplayText\(providerRequestId\)/);
assert.match(providerDiagnostics, /containsOpaqueUUID\(providerRequestId\)/);
assert.doesNotMatch(providerDiagnostics, />\s*\{providerRequestId\}\s*</);

const principalPicker = source('src/components/integrations/grant-principal-picker.tsx');
assert.doesNotMatch(principalPicker, /member_name\s*\|\|[\s\S]{0,100}member\.id/);
assert.doesNotMatch(principalPicker, /workspace\.name\s*\|\|\s*workspace\.id/);
assert.doesNotMatch(principalPicker, /selectedLabel\s*\|\|\s*initialLabel/);
assert.match(principalPicker, /safeInitialLabel/);
assert.match(principalPicker, /grants\.principalPicker\.unnamed\.account/);
assert.match(principalPicker, /grants\.principalPicker\.unnamed\.workspace/);

const connectedApps = source('src/components/chat/variants/aichat/connected-apps-dialog.tsx');
assert.match(connectedApps, /safeIntegrationDisplayText\(\s*connection\.name/);
assert.match(connectedApps, /safeOptionalIntegrationDisplayText\(connection\.display_name\)/);
assert.doesNotMatch(connectedApps, />\{connection\.name\}</);

const connectionDialog = source('src/components/integrations/connection-dialog.tsx');
assert.match(connectionDialog, /metadata\.providerName\(item\)/);
assert.match(connectionDialog, /safeOptionalIntegrationDisplayText\(connection\?\.name\)/);

const agentIntegrationDialog = source('src/components/agents/agent-runtime/integration-dialog.tsx');
assert.doesNotMatch(agentIntegrationDialog, /\{candidate\.integration_id\}/);
assert.doesNotMatch(agentIntegrationDialog, /\{candidate\.name\}/);
assert.match(agentIntegrationDialog, /integrationMetadata\.providerName\(/);

const agentIntegrationSection = source(
  'src/components/agents/agent-runtime/sections/integration-section.tsx'
);
assert.doesNotMatch(agentIntegrationSection, /integration:\s*binding\.integration_id/);
assert.doesNotMatch(agentIntegrationSection, />\s*\{actionId\}\s*</);
assert.match(agentIntegrationSection, /integrationMetadata\.actionName\(/);

const agentBindingHealth = source('src/components/agents/agent-runtime/binding-health.tsx');
assert.match(agentBindingHealth, /item\.binding_type === 'integration_connection'/);
assert.match(agentBindingHealth, /safeIntegrationDisplayText\(item\.display_name/);
assert.match(agentBindingHealth, /!isIntegrationConnection/);

const publishedVersions = source(
  'src/components/agents/agent-runtime/published-versions-dialog.tsx'
);
assert.match(publishedVersions, /item\.binding_type === 'integration_connection'/);
assert.match(publishedVersions, /safeIntegrationDisplayText\(\s*item\.display_name/);

const agentRuntimeModel = source(
  'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'
);
assert.doesNotMatch(
  agentRuntimeModel,
  /out\.add\(`Integration:\$\{binding\.integration_id\}:\$\{binding\.connection_id\}`\)/
);
assert.match(agentRuntimeModel, /labels\.integrationConnection/);
assert.match(agentRuntimeModel, /labels\.integrationAction/);
assert.doesNotMatch(agentRuntimeModel, /`Integration:\$\{integrationName\}`/);

const enMessages = source('src/i18n/modules/integrations/en-US.ts');
const zhMessages = source('src/i18n/modules/integrations/zh-Hans.ts');
for (const messages of [enMessages, zhMessages]) {
  assert.match(messages, /unknownExternalApp:/);
  assert.match(messages, /unnamedConnection:/);
  assert.match(messages, /unknownAction:/);
  assert.match(messages, /unknownConnection:/);
  assert.match(messages, /hiddenReference:/);
  assert.match(messages, /currentAccount:/);
}

const enAgentMessages = source('src/i18n/modules/agents/en-US.ts');
const zhAgentMessages = source('src/i18n/modules/agents/zh-Hans.ts');
for (const messages of [enAgentMessages, zhAgentMessages]) {
  assert.match(messages, /unnamedConnection:/);
  assert.match(messages, /unknownExternalApp:/);
  assert.match(messages, /unknownAction:/);
}

console.log('External app UUID display safety checks passed.');
