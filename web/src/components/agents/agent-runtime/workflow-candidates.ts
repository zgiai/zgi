import type {
  AgentWorkflowBinding,
  AgentWorkflowBindingCandidate,
} from '@/services/types/agent';

export function indexWorkflowBindingCandidates(
  candidates: AgentWorkflowBindingCandidate[],
  bindings: AgentWorkflowBinding[]
): Map<string, AgentWorkflowBindingCandidate> {
  const indexed = new Map(candidates.map(candidate => [candidate.binding_id, candidate]));
  for (const binding of bindings) {
    // Binding IDs are caller-defined aliases. Match the authorized candidate's
    // resource and version rather than assuming its suggested alias was saved.
    const candidate = candidates.find(
      item =>
        item.agent_id === binding.agent_id &&
        item.workflow_id === binding.workflow_id &&
        (!binding.version_uuid || item.version_uuid === binding.version_uuid)
    );
    if (candidate) {
      indexed.set(binding.binding_id, { ...candidate, ...binding });
    } else {
      indexed.delete(binding.binding_id);
    }
  }
  return indexed;
}
