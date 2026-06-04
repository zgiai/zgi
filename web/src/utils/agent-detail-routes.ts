import { AgentType } from '@/services/types/agent';

export type AgentDetailType = AgentType | string | null | undefined;

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

export function canShowWorkflowDetailPages(agentType: AgentDetailType): boolean {
  return isWorkflowRuntimeType(agentType);
}

export function canShowAgentApiKeys(agentType: AgentDetailType): boolean {
  return isWorkflowRuntimeType(agentType);
}

export function canShowAgentRuntimeLogs(agentType: AgentDetailType): boolean {
  return isWorkflowRuntimeType(agentType);
}
