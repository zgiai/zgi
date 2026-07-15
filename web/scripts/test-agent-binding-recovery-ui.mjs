import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const types = read('src/services/types/agent.ts');
const service = read('src/services/agent.service.ts');
const pageModel = read(
  'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'
);
const bindingHealth = read('src/components/agents/agent-runtime/binding-health.tsx');
const versionPopover = read('src/components/agents/agent-runtime/published-versions-dialog.tsx');
const databaseDialog = read('src/components/agents/agent-runtime/database-dialog.tsx');
const skillDialog = read('src/components/agents/agent-runtime/skill-dialog.tsx');
const knowledgeDialog = read('src/components/agents/agent-runtime/knowledge-dialog.tsx');
const workflowDialog = read('src/components/agents/agent-runtime/workflow-dialog.tsx');
const orchestrationPanel = read('src/components/agents/agent-runtime/orchestration-panel.tsx');
const dialogs = read('src/components/agents/agent-runtime/dialogs.tsx');
const previewPanel = read('src/components/agents/agent-runtime/preview-panel.tsx');
const draftPersistence = read(
  'src/components/agents/agent-runtime/use-agent-runtime-draft-persistence.ts'
);
const bindingRebaseMerge = read('src/components/agents/agent-runtime/binding-rebase-merge.ts');

assert.match(types, /binding_revision\?: string/);
assert.match(types, /binding_health\?: AgentBindingHealth/);
assert.match(types, /status: 'healthy' \| 'warning' \| 'blocked'/);
assert.match(types, /impact_token: string/);
assert.match(types, /binding_action: 'remove_all_abnormal'/);

assert.match(types, /acknowledge_suspended_bindings\?: boolean/);
assert.match(service, /published-versions\/\$\{versionId\}\/rollback-preview/);
assert.match(service, /agent_bindings_invalid/);
assert.match(service, /agent_bindings_suspended/);
assert.match(service, /agent_binding_revision_conflict/);
assert.match(service, /getAgentBindingRevisionConflict/);
assert.match(service, /getAgentRollbackImpactChanged/);
assert.match(service, /current_config \?\? nestedData\?\.config/);

assert.match(
  pageModel,
  /Array\.from\(new Set\(selectedSkillIds\.map\(id => id\.trim\(\)\)\.filter\(Boolean\)\)\)/
);
assert.doesNotMatch(pageModel, /selectedSkillIds\.filter\(id => selectableSkillIds\.has\(id\)\)/);
assert.doesNotMatch(pageModel, /const pruned = current\.filter/);
assert.match(pageModel, /handleRemoveAllAbnormalBindings/);
assert.match(pageModel, /acknowledge_suspended_bindings: acknowledgeSuspendedBindings/);
assert.match(pageModel, /focusInvalidBindings\(conflict\.bindingHealth\)/);
assert.match(pageModel, /impact_token: rollbackPreview\.impact_token/);
assert.match(pageModel, /binding_action: 'remove_all_abnormal'/);
assert.match(pageModel, /getAgentBindingRevisionConflict\(error\)/);
assert.match(pageModel, /enabled_skill_ids: serverConfig\.enabled_skill_ids \?\? \[\]/);
assert.match(pageModel, /binding_revision: serverConfig\.binding_revision/);
assert.match(
  pageModel,
  /response = await agentService\.updateAgentConfig\(agentId, configPayload\)/
);
assert.match(
  pageModel,
  /if \(result\.bindingPayloadRebased\) \{\s*applyServerBindingPayload\(result\.savedPayload, result\.bindingHealth\)/
);
assert.match(
  pageModel,
  /onSaveSuperseded: \(result, submittedPayload, latestPayload\) => \{\s*const mergedPayload = mergeSupersededAgentRuntimePayload\(\s*submittedPayload,\s*latestPayload,\s*result\.savedPayload\s*\);\s*applyServerBindingPayload\(mergedPayload, result\.bindingHealth\)/
);
assert.doesNotMatch(
  pageModel,
  /applyServerBindingPayload\(configPayload, conflict\?\.bindingHealth\)/
);
assert.doesNotMatch(pageModel, /if \(wasBindingRevisionRebased\) \{\s*applyServerBindingPayload/);
assert.match(
  draftPersistence,
  /if \(currentSignatureRef\.current !== submittedSignature\) \{\s*onSaveSupersededRef\.current\?\.\(result, submittedPayload, currentPayloadRef\.current\);\s*lastSavedSignatureRef\.current = savedSignature;\s*setLastSavedAt\(result\.updatedAt\);\s*setSaveState\('dirty'\);\s*return false;/
);
assert.doesNotMatch(draftPersistence, /currentSignatureRef\.current !== savedSignature/);
assert.match(bindingRebaseMerge, /mergeIDDelta\(/);
assert.match(bindingRebaseMerge, /mergeDatabaseBindingDelta\(/);
assert.match(bindingRebaseMerge, /mergeWorkflowBindingDelta\(/);
assert.doesNotMatch(pageModel, /setBindingHealth\(\{\s*status: 'healthy'/);
assert.match(pageModel, /setIsAbnormalBindingCleanupPending\(true\)/);
assert.match(pageModel, /getAgentRollbackImpactChanged\(error\)/);
assert.match(pageModel, /describeBindingChanges\(payload, serverConfig\)/);
assert.match(pageModel, /setCleanupBindingsDialogOpen\(true\)/);

assert.doesNotMatch(databaseDialog, /selectedDbIds\.filter\(dbId => scopedDbIds\.has\(dbId\)\)/);
assert.match(databaseDialog, /onConfirmDatabases\(\s*selectedDbIds,/);
assert.match(
  databaseDialog,
  /setSelectedDbIds\(bindings\.map\(binding => binding\.data_source_id\)\)/
);
assert.match(skillDialog, /setLocalSelectedSkillIds\(normalizedSelectedSkillIds\)/);
assert.match(knowledgeDialog, /setLocalSelectedDatasetIds\(selectedDatasetIds\)/);
assert.match(workflowDialog, /const existing = new Map\(bindings\.map/);
assert.match(workflowDialog, /if \(current\) return \[current\]/);
assert.match(orchestrationPanel, /bindingHealth=\{bindingHealth\}/);
assert.match(dialogs, /cleanupBindings/);
assert.match(previewPanel, /previewIgnoredDescription/);

assert.match(bindingHealth, /RemoveAllAbnormal|onRemoveAllAbnormal/);
assert.match(bindingHealth, /AgentSuspendedBindingsDialog/);
assert.match(versionPopover, /confirmCleanup/);
assert.match(versionPopover, /rollbackPreview\?\.impact_token/);

console.log('Agent binding recovery UI regression checks passed.');
