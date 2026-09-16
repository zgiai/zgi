import type {
  ConnectorTargetBinding,
  PersonalConnection,
  PersonalConnectionDetail,
  RemovalImpact,
  RenamePersonalConnectionRequest,
} from '@/services/types/connector';

export type ConnectionManagementPageState =
  | 'disabled'
  | 'loading'
  | 'error'
  | 'not_found'
  | 'ready'
  | 'reconnect_required'
  | 'unavailable';

interface ConnectionManagementPageStateInput {
  enabled: boolean;
  featureLoading?: boolean;
  isLoading?: boolean;
  error?: unknown;
  connection?: PersonalConnectionDetail | null;
}

interface RequestErrorLike {
  status?: unknown;
  response?: { status?: unknown };
}

function requestStatus(error: unknown): number | undefined {
  const candidate = (error ?? {}) as RequestErrorLike;
  const status = candidate.response?.status ?? candidate.status;
  return typeof status === 'number' ? status : undefined;
}

export function getConnectionManagementPageState({
  enabled,
  featureLoading = false,
  isLoading = false,
  error,
  connection,
}: ConnectionManagementPageStateInput): ConnectionManagementPageState {
  if (featureLoading) return 'loading';
  if (!enabled) return 'disabled';
  if (isLoading) return 'loading';
  if (error) return requestStatus(error) === 404 ? 'not_found' : 'error';
  return connection?.state ?? 'not_found';
}

const BINDING_STATUS_ORDER: Record<ConnectorTargetBinding['status'], number> = {
  active: 0,
  disabled: 1,
  needs_replacement: 2,
};

const TARGET_TYPE_ORDER: Record<ConnectorTargetBinding['target']['type'], number> = {
  chat: 0,
  agent: 1,
  workflow: 2,
  digital_human: 3,
};

export function sortConnectionBindings(
  bindings: readonly ConnectorTargetBinding[]
): ConnectorTargetBinding[] {
  return [...bindings].sort((left, right) => {
    const statusDifference = BINDING_STATUS_ORDER[left.status] - BINDING_STATUS_ORDER[right.status];
    if (statusDifference !== 0) return statusDifference;
    const typeDifference =
      TARGET_TYPE_ORDER[left.target.type] - TARGET_TYPE_ORDER[right.target.type];
    if (typeDifference !== 0) return typeDifference;
    const nameDifference = (left.target_name ?? '').localeCompare(right.target_name ?? '');
    return nameDifference || left.id.localeCompare(right.id);
  });
}

function queryString(values: Record<string, string | undefined>): string {
  const text = Object.entries(values)
    .filter((entry): entry is [string, string] => Boolean(entry[1]))
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
    .join('&');
  return text ? `?${text}` : '';
}

export function connectorBindingTargetHref(binding: ConnectorTargetBinding): string {
  const targetID = encodeURIComponent(binding.target.id);
  if (binding.target.type === 'chat') {
    return `/console/work/chat${queryString({ convId: binding.target.id })}`;
  }
  if (binding.target.type === 'agent') return `/console/agents/${targetID}`;
  if (binding.target.type === 'digital_human') {
    return `/console/digital-humans/${targetID}/configuration`;
  }
  return `/console/workflows/${targetID}${queryString({
    nodeId: binding.target.binding_instance_id,
  })}`;
}

export type ConnectionRemovalMode = 'normal' | 'force' | 'blocked';

export function getConnectionRemovalMode(
  impact: RemovalImpact,
  now = Date.now()
): ConnectionRemovalMode {
  if (!Array.isArray(impact.affected_targets) || typeof impact.can_remove !== 'boolean') {
    return 'blocked';
  }
  if (impact.affected_targets.length === 0 && impact.can_remove) return 'normal';
  const expiresAt = impact.expires_at ? Date.parse(impact.expires_at) : undefined;
  const tokenIsCurrent = expiresAt === undefined || (Number.isFinite(expiresAt) && expiresAt > now);
  if (
    impact.affected_targets.length > 0 &&
    Boolean(impact.impact_token?.trim()) &&
    tokenIsCurrent
  ) {
    return 'force';
  }
  return 'blocked';
}

export function buildRenameConnectionRequest(
  connection: Pick<PersonalConnection, 'revision'>,
  name: string
): RenamePersonalConnectionRequest | null {
  const normalizedName = name.trim();
  if (!normalizedName || normalizedName.length > 128) return null;
  return { revision: connection.revision, name: normalizedName };
}
