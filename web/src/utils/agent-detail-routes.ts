import { AgentType } from '@/services/types/agent';

export type AgentDetailType = AgentType | string | null | undefined;

export interface AgentDetailRoutePermissions {
  canView: boolean;
  canManage: boolean;
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

export function canShowWorkflowDetailPages(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return supportsWorkflowDetailPages(agentType) && permissions.canManage;
}

export function canShowAgentApiKeys(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return canShowWorkflowDetailPages(agentType, permissions);
}

export function canShowAgentRuntimeLogs(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return canShowWorkflowDetailPages(agentType, permissions);
}

export function canShowAgentBatchTest(
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
): boolean {
  return canShowWorkflowDetailPages(agentType, permissions);
}

export function getAgentDetailRouteAccess(
  agentId: string,
  agentType: AgentDetailType,
  permissions: AgentDetailRoutePermissions
) {
  const supportsWorkflowPages = supportsWorkflowDetailPages(agentType);
  const canManageWorkflowPages = supportsWorkflowPages && permissions.canManage;

  return {
    editHref: getAgentDetailEditHref(agentId, agentType),
    canView: permissions.canView,
    canManage: permissions.canManage,
    canEditRuntime: permissions.canManage,
    supportsWorkflowPages,
    canShowApiKeys: canManageWorkflowPages,
    canShowRuntimeLogs: canManageWorkflowPages,
    canShowBatchTest: canManageWorkflowPages,
  };
}
