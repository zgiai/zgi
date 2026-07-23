import type { AgentResourceBoundImpact } from '@/services/types/common';

export function getAgentResourceBoundImpact(error: unknown): AgentResourceBoundImpact | null {
  if (!error || typeof error !== 'object') return null;
  const responseData = (error as { response?: { data?: unknown } }).response?.data;
  if (!responseData || typeof responseData !== 'object') return null;
  const body = responseData as { code?: unknown; data?: unknown };
  if (body.code !== 'agent_resource_bound' || !body.data || typeof body.data !== 'object') {
    return null;
  }
  const impact = body.data as Partial<AgentResourceBoundImpact>;
  if (
    impact.code !== 'agent_resource_bound' ||
    typeof impact.operation !== 'string' ||
    typeof impact.resource_id !== 'string' ||
    typeof impact.impact_token !== 'string' ||
    !Array.isArray(impact.agents)
  ) {
    return null;
  }
  return impact as AgentResourceBoundImpact;
}
