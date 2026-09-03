import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const source = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const panel = source('src/components/integrations/connections-panel.tsx');
const impactDetails = source('src/components/integrations/connection-reference-impact.tsx');
const service = source('src/services/integration.service.ts');
const types = source('src/services/types/integration.ts');
const hooks = source('src/hooks/integrations/use-integrations.ts');
const english = source('src/i18n/modules/integrations/en-US.ts');
const chinese = source('src/i18n/modules/integrations/zh-Hans.ts');

assert.match(panel, /const refreshDeleteImpact = useCallback/);
assert.match(
  panel,
  /deleteImpactRequestRef\.current !== requestID/,
  'stale impact responses must not replace a newer dependency snapshot'
);
assert.match(
  panel,
  /window\.addEventListener\('focus', refreshOnFocus\)/,
  'returning from an Agent editor must refresh the open dependency dialog'
);
assert.match(panel, /t\('delete\.refreshImpact'\)/);
assert.match(panel, /<ConnectionReferenceImpact/);
assert.match(panel, /detachReferencesMutation\s*\.mutateAsync/);
assert.match(panel, /data: \{ impact_token: deleteImpact\.impact_token \}/);
assert.match(
  panel,
  /deleteImpactRequestRef\.current \+= 1;[\s\S]*detachReferencesMutation[\s\S]*setDeleteImpact\(response\.data\)/,
  'detach success must invalidate older impact GET responses before applying its result'
);
assert.match(
  panel,
  /\.then\(response => \{[\s\S]*deleteImpactRequestRef\.current \+= 1;[\s\S]*setDeleteImpact\(response\.data\)/,
  'detach success must also invalidate impact GET responses started while the POST was in flight'
);
assert.match(panel, /\.catch\(\(\) => \{[\s\S]*refreshDeleteImpact\(deleteConnection\)/);
assert.match(impactDetails, /groupReferencesByWorkspace/);
assert.match(impactDetails, /reference\.draft_referenced/);
assert.match(impactDetails, /reference\.published_referenced/);
assert.match(impactDetails, /reference\.nodes\.map/);
assert.match(impactDetails, /metadata\.actionName/);
assert.match(impactDetails, /\/console\/workflows\/\$\{reference\.asset_id\}/);
assert.match(impactDetails, /\/console\/agents\/\$\{reference\.asset_id\}/);
assert.doesNotMatch(
  impactDetails,
  />\s*\{(?:reference\.asset_id|reference\.workspace_id|node\.node_id|actionID)\}\s*</,
  'opaque resource identifiers must not be reader-visible'
);
assert.match(types, /bound_workflow_count: number/);
assert.match(types, /asset_type: 'agent' \| 'workflow'/);
assert.match(types, /nodes\?: IntegrationConnectionReferenceNode\[\]/);
assert.match(types, /impact_token\?: string/);
assert.match(service, /\/integrations\/connections\/\$\{id\}\/detach-references/);
assert.match(hooks, /useDetachIntegrationConnectionReferences/);
assert.match(hooks, /queryKey: AGENT_KEYS\.all/);
assert.match(english, /refreshImpact: 'Refresh references'/);
assert.match(chinese, /refreshImpact: '刷新引用关系'/);
assert.match(english, /referencesDetached:/);
assert.match(chinese, /referencesDetached:/);

console.log('Integration connection dependency refresh checks passed.');
