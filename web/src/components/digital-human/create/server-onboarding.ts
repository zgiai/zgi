import { connectorService } from '@/services/connector.service';
import { digitalHumanService } from '@/services/digital-human.service';
import { connectorOperationIds } from '../digital-human-connector-model';
import type { DigitalHumanCreateConnectorSelection } from './digital-human-connector-create-model';

/** Operations a scheduled inspection may use (ADR 0027): reads only. */
const INSPECTION_OPERATION_IDS = [
  'server.system.snapshot',
  'server.service.status',
  'server.journal.tail',
  'server.command.run',
];

/** How long and how often the one-click daily inspection may run. */
const INSPECTION_GRANT_DAYS = 90;
const INSPECTION_GRANT_CALLS = 2000;

export interface DailyInspectionCopy {
  name: string;
  goal: string;
  criteria: string[];
}

/**
 * Turn on a daily inspection for a freshly published operations specialist:
 * a bounded task grant for the server's read operations on its published
 * binding, then a task every morning. Each call is checked by the platform
 * again when it runs; a change still waits for a person.
 */
export async function enableDailyServerInspection(
  digitalHumanId: string,
  serverConnectorId: string,
  selection: DigitalHumanCreateConnectorSelection,
  copy: DailyInspectionCopy,
  timezone: string
): Promise<void> {
  const detail = await digitalHumanService.get(digitalHumanId);
  const versionId = detail.data?.definition.published_version_id;
  if (!versionId) throw new Error('digital human is not published');
  const bindings = await connectorService.getTargetBindings({
    type: 'digital_human',
    id: digitalHumanId,
    scope: 'published',
    version_id: versionId,
  });
  const binding = (bindings.data?.items ?? []).find(
    item =>
      item.status === 'active' &&
      item.connector_id === serverConnectorId &&
      item.connection_id === selection.binding.resource_id
  );
  if (!binding) throw new Error('published server binding not found');
  const selected = connectorOperationIds(selection.binding);
  const operations = INSPECTION_OPERATION_IDS.filter(id => selected.includes(id));
  if (operations.length === 0) throw new Error('no inspection operations');
  await connectorService.issueBackgroundGrant(binding.id, {
    operations,
    expires_at: new Date(Date.now() + INSPECTION_GRANT_DAYS * 24 * 60 * 60 * 1000).toISOString(),
    max_invocations: INSPECTION_GRANT_CALLS,
  });
  await digitalHumanService.createTask(digitalHumanId, {
    name: copy.name,
    goal: copy.goal,
    success_criteria: copy.criteria,
    schedule: { type: 'cron', cron: '0 9 * * *', timezone },
  });
}
