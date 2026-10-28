import type {
  ConnectorBackgroundGrant,
  IssueConnectorBackgroundGrantRequest,
} from '@/services/types/connector';

export interface BackgroundGrantScope {
  organizationId: string;
  workspaceId: string;
  accountId: string;
  employeeId: string;
  versionId: string;
}

export interface BackgroundGrantBindingSnapshot {
  id: string;
  connection_id: string;
  revision: number;
  status: string;
}

export interface BackgroundGrantConnectionSnapshot {
  id: string;
  revision: number;
  credential_version?: number;
}

export interface BackgroundGrantDestinationRow {
  field: string;
  value: string;
}

export interface BackgroundGrantDraft {
  operations: string[];
  destinationRows: BackgroundGrantDestinationRow[];
  durationDays: number;
  maxInvocations: number;
}

export type BackgroundGrantDraftError =
  | 'operations'
  | 'duration'
  | 'maxInvocations'
  | 'destinations';

export type BackgroundGrantDraftResult =
  | { request: IssueConnectorBackgroundGrantRequest }
  | { error: BackgroundGrantDraftError };

export interface BackgroundGrantRow {
  grant: ConnectorBackgroundGrant;
  stale: boolean;
  expired: boolean;
  revoked: boolean;
}

export function backgroundGrantQueryKey(scope: BackgroundGrantScope) {
  return [
    'connector-background-grants',
    scope.organizationId,
    scope.workspaceId,
    scope.accountId,
    scope.employeeId,
    scope.versionId,
  ] as const;
}

export function buildBackgroundGrantIssueRequest(
  draft: BackgroundGrantDraft,
  now = new Date()
): BackgroundGrantDraftResult {
  if (draft.operations.length === 0 || new Set(draft.operations).size !== draft.operations.length) {
    return { error: 'operations' };
  }
  if (!Number.isInteger(draft.durationDays) || draft.durationDays < 1 || draft.durationDays > 90) {
    return { error: 'duration' };
  }
  if (
    !Number.isInteger(draft.maxInvocations) ||
    draft.maxInvocations < 1 ||
    draft.maxInvocations > 100_000
  ) {
    return { error: 'maxInvocations' };
  }
  if (draft.destinationRows.length > 16) return { error: 'destinations' };
  const destinations: Record<string, string> = {};
  for (const row of draft.destinationRows) {
    const field = row.field.trim();
    const value = row.value.trim();
    if (
      !/^[A-Za-z_][A-Za-z0-9_.-]{0,63}$/.test(field) ||
      !value ||
      value.length > 512 ||
      value.includes(String.fromCharCode(0)) ||
      value.includes('\r') ||
      value.includes('\n') ||
      value.includes('*') ||
      Object.hasOwn(destinations, field)
    ) {
      return { error: 'destinations' };
    }
    destinations[field] = value;
  }
  const expiresAt = new Date(now.getTime() + draft.durationDays * 24 * 60 * 60 * 1000);
  return {
    request: {
      operations: [...draft.operations],
      ...(Object.keys(destinations).length > 0 ? { destinations } : {}),
      expires_at: expiresAt.toISOString(),
      max_invocations: draft.maxInvocations,
    },
  };
}

export function grantsForEmployee(
  grants: ConnectorBackgroundGrant[],
  scope: BackgroundGrantScope,
  bindings: BackgroundGrantBindingSnapshot[],
  connections: BackgroundGrantConnectionSnapshot[],
  now = new Date()
): BackgroundGrantRow[] {
  return grants
    .filter(grant => grant.employee_id === scope.employeeId)
    .map(grant => {
      const binding = bindings.find(value => value.id === grant.binding_id);
      const connection = connections.find(value => value.id === grant.connection_id);
      const stale =
        grant.version_id !== scope.versionId ||
        !binding ||
        binding.status !== 'active' ||
        binding.connection_id !== grant.connection_id ||
        binding.revision !== grant.binding_revision ||
        !connection ||
        connection.revision !== grant.connection_revision ||
        connection.credential_version !== grant.credential_version;
      return {
        grant,
        stale,
        expired: new Date(grant.expires_at).getTime() <= now.getTime(),
        revoked: Boolean(grant.revoked_at),
      };
    })
    .sort((left, right) => {
      if (left.stale !== right.stale) return left.stale ? 1 : -1;
      return Date.parse(right.grant.created_at) - Date.parse(left.grant.created_at);
    });
}

/**
 * Server changes always need a person, so a task grant for SSH Server
 * Operations covers inspection only; each command must also prove read-only
 * when it runs (ADR 0027).
 */
const UNATTENDED_SSH_OPERATION_IDS: ReadonlySet<string> = new Set([
  'server.system.snapshot',
  'server.service.status',
  'server.journal.tail',
  'server.command.run',
]);

export function backgroundGrantOperationAllowed(connectorId: string, operationId: string) {
  return connectorId !== 'serverops_ssh' || UNATTENDED_SSH_OPERATION_IDS.has(operationId);
}
