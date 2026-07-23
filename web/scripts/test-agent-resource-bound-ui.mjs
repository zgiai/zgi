import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const parser = read('src/utils/agent-resource-bound.ts');
const dialog = read('src/components/common/agent-resource-bound-dialog.tsx');
const datasetService = read('src/services/dataset.service.ts');
const datasetCard = read('src/components/datasets/dataset-card.tsx');
const skillService = read('src/services/aichat.service.ts');
const skillSettings = read(
  'src/components/dashboard/organization/aichat-skill-settings-section.tsx'
);
const moveDialog = read('src/components/common/workspace-asset-move-dialog.tsx');
const moveHook = read('src/hooks/organization/use-workspace-asset-move.ts');
const organizationService = read('src/services/organization.service.ts');
const workspaceSelector = read('src/components/common/workspace-selector/index.tsx');
const workspaceManagementPage = read('src/app/dashboard/organization/workspaces/page.tsx');
const organizationTypes = read('src/services/types/organization.ts');
const dbService = read('src/services/db.service.ts');
const dbCard = read('src/components/db/card/index.tsx');
const dbTableLayout = read('src/app/console/db/[dbId]/layout.tsx');
const agentCard = read('src/components/agents/agent-card.tsx');
const agentService = read('src/services/agent.service.ts');
const aichatTypes = read('src/services/types/aichat.ts');

assert.match(parser, /body\.code !== 'agent_resource_bound'/);
assert.match(parser, /impact\.impact_token/);
assert.match(dialog, /IconPreview/);
assert.match(dialog, /agent\.name/);
assert.match(dialog, /agent\.description/);
assert.match(dialog, /`\/console\/agents\/\$\{agent\.agent_id\}`/);
assert.doesNotMatch(dialog, /agent\.draft/);
assert.doesNotMatch(dialog, /agent\.published/);
assert.doesNotMatch(dialog, /agent\.binding_count/);

assert.match(datasetService, /params: confirmation/);
assert.match(datasetService, /previewDatasetDeleteImpact/);
assert.match(datasetCard, /agent_binding_action: 'unbind'/);
assert.match(datasetCard, /getAgentResourceBoundImpact\(error\)/);
assert.match(datasetCard, /previewDatasetDeleteImpact/);

assert.match(skillService, /deleteSkill\(id: string, confirmation\?/);
assert.match(skillService, /previewSkillDeleteImpact/);
assert.match(skillService, /AIChatSkillConfigUpdateResponse/);
assert.match(skillSettings, /agent_binding_action: 'unbind'/);
assert.match(skillSettings, /AgentResourceBoundDialog/);
assert.match(skillSettings, /previewSkillDeleteImpact/);

assert.match(organizationTypes, /agent_binding_action\?: 'unbind'/);
assert.match(organizationTypes, /agent_binding_impact\?: AgentResourceBoundImpact/);
assert.match(moveDialog, /impact_token: impact\?\.impact_token/);
assert.match(moveDialog, /handleSubmit\(bindingImpact\)/);
assert.match(moveDialog, /setDependencyImpact\(nextImpact\)/);
assert.match(moveDialog, /setPreflightBindingOpen\(Boolean\(nextImpact\?\.agents\.length\)\)/);
assert.match(moveDialog, /bindingConfirmOpen/);
assert.match(moveDialog, /setBindingConfirmOpen\(true\)/);
assert.match(moveDialog, /open=\{bindingConfirmOpen && Boolean\(bindingImpact\)\}/);
assert.match(moveDialog, /!bindingImpact &&/);
assert.match(moveDialog, /createWorkspace=1/);
assert.match(moveDialog, /organizationRole === 'owner' \|\| organizationRole === 'admin'/);
assert.match(workspaceManagementPage, /params\.get\('createWorkspace'\) !== '1'/);
assert.match(workspaceManagementPage, /mode: 'create'/);
assert.match(workspaceManagementPage, /params\.delete\('createWorkspace'\)/);

assert.match(dbService, /deleteDb\([\s\S]*confirmation\?: AgentBindingMutationConfirmation/);
assert.match(dbService, /deleteDbTable\([\s\S]*confirmation\?: AgentBindingMutationConfirmation/);
assert.match(dbService, /previewDbDeleteImpact/);
assert.match(dbService, /previewDbTableDeleteImpact/);
assert.match(dbCard, /agent_binding_action: 'unbind'/);
assert.match(dbCard, /getAgentResourceBoundImpact\(error\)/);
assert.match(dbTableLayout, /agent_binding_action: 'unbind'/);
assert.match(dbTableLayout, /getAgentResourceBoundImpact\(error\)/);
assert.match(dbTableLayout, /previewDbTableDeleteImpact/);
assert.match(agentService, /previewAgentDeleteImpact/);
assert.match(agentCard, /agent_binding_action: 'unbind'/);
assert.match(agentCard, /AgentResourceBoundDialog/);
assert.match(agentCard, /previewAgentDeleteImpact/);
assert.match(aichatTypes, /agent_binding_action\?: 'retain_suspended'/);
assert.match(aichatTypes, /status: 'confirmation_required'/);
assert.match(aichatTypes, /applied: false/);
assert.match(skillSettings, /agent_binding_action: impact \? 'retain_suspended' : undefined/);
assert.match(skillSettings, /impact_token: impact\?\.impact_token/);
assert.doesNotMatch(skillSettings, /AUTO_SAVE_DELAY_MS/);
assert.doesNotMatch(skillSettings, /useAIChatSkillConfigAutosave/);
const skillConfigPersistenceStart = skillSettings.indexOf(
  'function useAIChatSkillConfigPersistence'
);
const nextSkillSettingsSectionStart = skillSettings.indexOf(
  'interface SkillImportPreviewDialogProps',
  skillConfigPersistenceStart
);
const skillConfigPersistenceBlock = skillSettings.slice(
  skillConfigPersistenceStart,
  nextSkillSettingsSectionStart
);
assert.ok(
  skillConfigPersistenceStart !== -1 && nextSkillSettingsSectionStart > skillConfigPersistenceStart
);
assert.match(skillConfigPersistenceBlock, /await save\(requestedSkillIds, impact\)/);
assert.match(skillConfigPersistenceBlock, /if \(!result\.applied\)/);
assert.match(
  skillConfigPersistenceBlock,
  /onConfirmationRequired\(result\.impact, requestedSkillIds\)/
);
assert.match(skillConfigPersistenceBlock, /setEnabledSkillIds\(savedSkillIds\)/);
assert.ok(
  skillConfigPersistenceBlock.indexOf('await save(requestedSkillIds, impact)') <
    skillConfigPersistenceBlock.indexOf('setEnabledSkillIds(savedSkillIds)'),
  'Skill switches must update only after the server accepts the policy change'
);
const skillToggleStart = skillSettings.indexOf('const handleToggle =');
const skillToggleEnd = skillSettings.indexOf(
  'const handleConfirmRetainSuspended',
  skillToggleStart
);
const skillToggleBlock = skillSettings.slice(skillToggleStart, skillToggleEnd);
assert.match(skillToggleBlock, /saveEnabledSkillIds\(Array\.from\(next\)\)/);
assert.doesNotMatch(skillToggleBlock, /setEnabledSkillIds/);
assert.match(skillSettings, /retainSuspendedWarningDescription/);
assert.match(moveDialog, /setBindingImpact\(nextImpact\)/);
assert.match(moveDialog, /agent_binding_action: impact \? 'unbind' : undefined/);
assert.match(moveDialog, /useWorkspaceAssetMoveEligibleTargets\(moveItems, open\)/);
assert.match(moveDialog, /toast\.loading\(t\('assetMove\.preflightChecking'\)/);
assert.match(
  moveDialog,
  /dependencyMutationRef\.current[\s\S]*?\.mutateAsync\(\{ items: moveItems \}\)/
);
assert.match(
  moveDialog,
  /open=\{open && preflightReady && !preflightBindingOpen && !bindingConfirmOpen\}/
);
assert.match(moveDialog, /agents=\{dependencyImpact\?\.agents\}/);
assert.match(moveDialog, /actionLabel=\{t\('assetMove\.continueToTargetSelection'\)\}/);
assert.match(moveDialog, /actionVariant="default"/);
assert.match(moveDialog, /setAcknowledgedDependencyAgentIds/);
assert.match(moveDialog, /workspaceOptions=\{eligibleTargets\}/);
assert.doesNotMatch(moveDialog, /useWorkspaces\(/);
assert.match(moveHook, /'workspace-asset-move',\s*'eligible-targets'/);
assert.match(moveHook, /Math\.ceil\(firstPage\.total \/ limit\)/);
assert.match(organizationService, /\/organizations\/current\/assets\/move\/eligible-targets/);
assert.match(organizationService, /\/organizations\/current\/assets\/move\/dependencies/);
assert.match(workspaceSelector, /enabled: !hasExternalWorkspaceOptions/);
assert.match(workspaceSelector, /<Users className="h-4 w-4 shrink-0 opacity-70"/);
assert.match(workspaceSelector, /value && !isLoading/);
assert.doesNotMatch(workspaceSelector, /valueInOptions/);
assert.doesNotMatch(workspaceSelector, /Building2/);
assert.match(moveDialog, /<Users className="h-4 w-4 shrink-0 text-muted-foreground"/);
assert.match(moveDialog, /className="flex min-h-10 items-center gap-2 px-1 text-sm"/);
const confirmMoveStart = moveDialog.indexOf('const handleConfirmMove = async');
const confirmMoveEnd = moveDialog.indexOf('const handleTargetWorkspaceChange', confirmMoveStart);
const confirmMoveBlock = moveDialog.slice(confirmMoveStart, confirmMoveEnd);
assert.match(confirmMoveBlock, /await previewMutation\.mutateAsync\(request\)/);
const targetPreviewIndex = confirmMoveBlock.indexOf('await previewMutation.mutateAsync(request)');
const bindingConfirmationAfterPreview = confirmMoveBlock.indexOf(
  'setBindingConfirmOpen(true)',
  targetPreviewIndex
);
assert.ok(
  targetPreviewIndex !== -1 && bindingConfirmationAfterPreview > targetPreviewIndex,
  'move preview must finish before the binding confirmation dialog opens'
);
const targetChangeBlock = moveDialog.slice(
  confirmMoveEnd,
  moveDialog.indexOf('return (', confirmMoveEnd)
);
assert.doesNotMatch(
  targetChangeBlock,
  /previewMutation\.mutateAsync/,
  'selecting a target workspace must not run the target-specific preview'
);

console.log('Agent resource-bound lifecycle UI regression checks passed.');
