import type {
  AgentDatabaseBinding,
  AgentWorkflowBinding,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import { normalizeAgentDatabaseBindings } from './database-binding-draft';

function normalizeIDs(ids: string[] | undefined): string[] {
  return Array.from(new Set((ids ?? []).map(id => id.trim()).filter(Boolean)));
}

function mergeIDDelta(submitted: string[], current: string[], saved: string[]): string[] {
  const submittedSet = new Set(normalizeIDs(submitted));
  const currentIDs = normalizeIDs(current);
  const currentSet = new Set(currentIDs);
  const result = normalizeIDs(saved).filter(id => !submittedSet.has(id) || currentSet.has(id));
  const resultSet = new Set(result);

  currentIDs.forEach(id => {
    if (!submittedSet.has(id) && !resultSet.has(id)) {
      result.push(id);
      resultSet.add(id);
    }
  });
  return result;
}

export function normalizeAgentWorkflowBindings(
  bindings: AgentWorkflowBinding[]
): AgentWorkflowBinding[] {
  const byBindingID = new Map<string, AgentWorkflowBinding>();
  bindings.forEach(binding => {
    const bindingId = binding.binding_id.trim();
    const agentId = binding.agent_id.trim();
    const workflowId = binding.workflow_id.trim();
    if (!bindingId || !agentId || !workflowId) return;
    const versionStrategy = binding.version_strategy || 'latest_published';
    if (versionStrategy !== 'latest_published' && versionStrategy !== 'pinned') return;
    byBindingID.set(bindingId, {
      binding_id: bindingId,
      label: binding.label.trim(),
      description: binding.description?.trim() || undefined,
      agent_id: agentId,
      workflow_id: workflowId,
      agent_type: binding.agent_type,
      version_strategy: versionStrategy,
      version_uuid:
        versionStrategy === 'pinned' ? binding.version_uuid?.trim() || undefined : undefined,
      timeout_seconds: Math.max(0, binding.timeout_seconds ?? 0),
    });
  });
  return Array.from(byBindingID.values()).sort((left, right) =>
    left.binding_id.localeCompare(right.binding_id)
  );
}

function mergeDatabaseBindingDelta(
  submitted: AgentDatabaseBinding[],
  current: AgentDatabaseBinding[],
  saved: AgentDatabaseBinding[]
): AgentDatabaseBinding[] {
  const submittedMap = new Map(
    normalizeAgentDatabaseBindings(submitted).map(binding => [binding.data_source_id, binding])
  );
  const currentMap = new Map(
    normalizeAgentDatabaseBindings(current).map(binding => [binding.data_source_id, binding])
  );
  const resultMap = new Map(
    normalizeAgentDatabaseBindings(saved).map(binding => [binding.data_source_id, binding])
  );

  submittedMap.forEach((_, dataSourceID) => {
    if (!currentMap.has(dataSourceID)) resultMap.delete(dataSourceID);
  });

  currentMap.forEach((currentBinding, dataSourceID) => {
    const submittedBinding = submittedMap.get(dataSourceID);
    if (submittedBinding && JSON.stringify(submittedBinding) === JSON.stringify(currentBinding)) {
      return;
    }
    const savedBinding = resultMap.get(dataSourceID);
    const tableIDs = mergeIDDelta(
      submittedBinding?.table_ids ?? [],
      currentBinding.table_ids,
      savedBinding?.table_ids ?? []
    );
    const writableTableIDs = mergeIDDelta(
      submittedBinding?.writable_table_ids ?? [],
      currentBinding.writable_table_ids ?? [],
      savedBinding?.writable_table_ids ?? []
    );
    const readableSet = new Set(tableIDs);
    writableTableIDs.forEach(id => readableSet.add(id));
    if (readableSet.size === 0) {
      resultMap.delete(dataSourceID);
      return;
    }
    resultMap.set(dataSourceID, {
      data_source_id: dataSourceID,
      table_ids: Array.from(readableSet).sort(),
      writable_table_ids: writableTableIDs.filter(id => readableSet.has(id)).sort(),
    });
  });

  return Array.from(resultMap.values()).sort((left, right) =>
    left.data_source_id.localeCompare(right.data_source_id)
  );
}

function mergeWorkflowBindingDelta(
  submitted: AgentWorkflowBinding[],
  current: AgentWorkflowBinding[],
  saved: AgentWorkflowBinding[]
): AgentWorkflowBinding[] {
  const submittedMap = new Map(
    normalizeAgentWorkflowBindings(submitted).map(binding => [binding.binding_id, binding])
  );
  const currentMap = new Map(
    normalizeAgentWorkflowBindings(current).map(binding => [binding.binding_id, binding])
  );
  const resultMap = new Map(
    normalizeAgentWorkflowBindings(saved).map(binding => [binding.binding_id, binding])
  );

  submittedMap.forEach((_, bindingID) => {
    if (!currentMap.has(bindingID)) resultMap.delete(bindingID);
  });
  currentMap.forEach((currentBinding, bindingID) => {
    const submittedBinding = submittedMap.get(bindingID);
    if (!submittedBinding || JSON.stringify(submittedBinding) !== JSON.stringify(currentBinding)) {
      resultMap.set(bindingID, currentBinding);
    }
  });
  return Array.from(resultMap.values()).sort((left, right) =>
    left.binding_id.localeCompare(right.binding_id)
  );
}

// Use the server-saved payload as the new baseline and replay only edits made
// after this save started. Unchanged local bindings therefore cannot restore
// resources that another editor changed before the revision conflict.
export function mergeSupersededAgentRuntimePayload(
  submitted: UpdateAgentRuntimeConfigRequest,
  current: UpdateAgentRuntimeConfigRequest,
  saved: UpdateAgentRuntimeConfigRequest
): UpdateAgentRuntimeConfigRequest {
  return {
    ...saved,
    ...current,
    enabled_skill_ids: mergeIDDelta(
      submitted.enabled_skill_ids,
      current.enabled_skill_ids,
      saved.enabled_skill_ids
    ),
    knowledge_dataset_ids: mergeIDDelta(
      submitted.knowledge_dataset_ids ?? [],
      current.knowledge_dataset_ids ?? [],
      saved.knowledge_dataset_ids ?? []
    ),
    database_bindings: mergeDatabaseBindingDelta(
      submitted.database_bindings ?? [],
      current.database_bindings ?? [],
      saved.database_bindings ?? []
    ),
    workflow_bindings: mergeWorkflowBindingDelta(
      submitted.workflow_bindings ?? [],
      current.workflow_bindings ?? [],
      saved.workflow_bindings ?? []
    ),
    binding_revision: saved.binding_revision,
  };
}
