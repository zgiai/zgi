import { AgentType } from '@/services/types/agent';

export type AgentDetailType = AgentType | string | null | undefined;

export interface AgentDetailRoutePermissions {
  canView: boolean;
  canManage?: boolean;
  canEditRuntime?: boolean;
  canManageRuntimeAccess?: boolean;
  canViewRuntimeLogs?: boolean;
  canRunBatchTest?: boolean;
}

function normalizeAgentType(agentType: AgentDetailType): string {
  return String(agentType ?? '')
    .trim()
    .toUpperCase()
    .replace(/-/g, '_');
}

export function isAgentRuntimeType(agentType: AgentDetailType): boolean {
  return normalizeAgentType(agentType) === AgentType.AGENT;
}

export function isWorkflowRuntimeType(agentType: AgentDetailType): boolean {
  const type = normalizeAgentType(agentType);
  return (
    type === AgentType.WORKFLOW ||
    type === AgentType.CONVERSATIONAL_AGENT ||
    type === 'CONVERSATIONAL_AGENT'
  );
}

export function getAgentDetailEditHref(agentId: string, agentType: AgentDetailType): string {
  const editor = isAgentRuntimeType(agentType) ? 'agent' : 'workflow';
  return `/console/agents/${agentId}/${editor}`;
}

export function supportsWorkflowDetailPages(agentType: AgentDetailType): boolean {
  return isWorkflowRuntimeType(agentType);
}

export function supportsAgentRuntimeLogs(agentType: AgentDetailType): boolean {
  return isWorkflowRuntimeType(agentType) || isAgentRuntimeType(agentType);
}

export function canShowWorkflowDetailPages(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return (
    supportsWorkflowDetailPages(agentType) &&
    Boolean(permissions.canEditRuntime ?? permissions.canManage)
  );
}

export function canShowAgentApiKeys(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return (
    supportsWorkflowDetailPages(agentType) &&
    Boolean(permissions.canManageRuntimeAccess ?? permissions.canManage)
  );
}

export function canShowAgentRuntimeAccess(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return (
    supportsAgentRuntimeLogs(agentType) &&
    Boolean(permissions.canManageRuntimeAccess ?? permissions.canManage)
  );
}

export function canShowAgentRuntimeLogs(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return (
    supportsAgentRuntimeLogs(agentType) &&
    Boolean(permissions.canViewRuntimeLogs ?? permissions.canManage)
  );
}

export function canShowAgentBatchTest(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return (
    supportsWorkflowDetailPages(agentType) &&
    Boolean(permissions.canRunBatchTest ?? permissions.canManage)
  );
}

export function getAgentDetailRouteAccess(
  agentId: string,
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
) {
  const supportsWorkflowPages = supportsWorkflowDetailPages(agentType);
  const canManage = Boolean(permissions.canManage);
  const canEditRuntime = Boolean(permissions.canEditRuntime ?? permissions.canManage);
  const canManageRuntimeAccess = Boolean(
    permissions.canManageRuntimeAccess ?? permissions.canManage
  );
  const canViewRuntimeLogs = Boolean(permissions.canViewRuntimeLogs ?? permissions.canManage);
  const canRunBatchTest = Boolean(permissions.canRunBatchTest ?? permissions.canManage);

  return {
    editHref: getAgentDetailEditHref(agentId, agentType),
    canView: permissions.canView,
    canManage:
      canManage ||
      canEditRuntime ||
      canManageRuntimeAccess ||
      canViewRuntimeLogs ||
      canRunBatchTest,
    canEditRuntime,
    supportsWorkflowPages,
    canShowApiKeys: supportsWorkflowPages && canManageRuntimeAccess,
    canShowRuntimeAccess: canShowAgentRuntimeAccess(agentType, permissions),
    canShowRuntimeLogs: supportsAgentRuntimeLogs(agentType) && canViewRuntimeLogs,
    canShowBatchTest: supportsWorkflowPages && canRunBatchTest,
  };
}

export function getWebAppRunHref(webAppId: string, agentType: AgentDetailType): string {
  const type = normalizeAgentType(agentType);
  const mode = type === AgentType.AGENT || type === AgentType.CONVERSATIONAL_AGENT ? 'chat' : 'run';
  return `/webapp/${webAppId}/${mode}`;
}
