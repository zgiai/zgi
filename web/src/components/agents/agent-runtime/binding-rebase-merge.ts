import type {
  AgentConnectorBinding,
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

export function normalizeAgentConnectorBindings(
  bindings: AgentConnectorBinding[]
): AgentConnectorBinding[] {
  const byConnectorID = new Map<string, AgentConnectorBinding>();
  bindings.forEach(binding => {
    const connectorID = binding.connector_id.trim().toLowerCase();
    const connectionID = binding.connection_id.trim().toLowerCase();
    if (!connectorID || !connectionID) return;
    byConnectorID.set(connectorID, {
      ...(binding.binding_id ? { binding_id: binding.binding_id } : {}),
      ...(binding.revision !== undefined ? { revision: binding.revision } : {}),
      ...(binding.authorization_mode
        ? { authorization_mode: binding.authorization_mode }
        : {}),
      connector_id: connectorID,
      connection_id: connectionID,
    });
  });
  return Array.from(byConnectorID.values()).sort((left, right) =>
    left.connector_id.localeCompare(right.connector_id)
  );
}

// A confirmation is tied to the previously selected connection. Replaying it
// over a different server selection would silently confirm a different switch.
export function getAgentConnectorBindingConflicts(
  baseline: Pick<UpdateAgentRuntimeConfigRequest, 'connector_bindings'>,
  current: Pick<UpdateAgentRuntimeConfigRequest, 'connector_bindings'>,
  saved: Pick<UpdateAgentRuntimeConfigRequest, 'connector_bindings'>
): string[] {
  const byConnector = (payload: Pick<UpdateAgentRuntimeConfigRequest, 'connector_bindings'>) =>
    new Map(
      normalizeAgentConnectorBindings(payload.connector_bindings ?? []).map(binding => [
        binding.connector_id,
        binding.connection_id,
      ])
    );
  const baselineMap = byConnector(baseline);
  const currentMap = byConnector(current);
  const savedMap = byConnector(saved);
  const connectorIDs = new Set([...baselineMap.keys(), ...currentMap.keys(), ...savedMap.keys()]);
  return Array.from(connectorIDs)
    .filter(connectorID => {
      const baselineConnection = baselineMap.get(connectorID);
      const currentConnection = currentMap.get(connectorID);
      const savedConnection = savedMap.get(connectorID);
      return (
        currentConnection !== baselineConnection &&
        savedConnection !== baselineConnection &&
        currentConnection !== savedConnection
      );
    })
    .sort();
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

function mergeConnectorBindingDelta(
  submitted: AgentConnectorBinding[],
  current: AgentConnectorBinding[],
  saved: AgentConnectorBinding[]
): AgentConnectorBinding[] {
  const submittedMap = new Map(
    normalizeAgentConnectorBindings(submitted).map(binding => [binding.connector_id, binding])
  );
  const currentMap = new Map(
    normalizeAgentConnectorBindings(current).map(binding => [binding.connector_id, binding])
  );
  const resultMap = new Map(
    normalizeAgentConnectorBindings(saved).map(binding => [binding.connector_id, binding])
  );
  const conflictingConnectorIDs = new Set(
    getAgentConnectorBindingConflicts(
      { connector_bindings: submitted },
      { connector_bindings: current },
      { connector_bindings: saved }
    )
  );

  submittedMap.forEach((_, connectorID) => {
    if (!currentMap.has(connectorID) && !conflictingConnectorIDs.has(connectorID)) {
      resultMap.delete(connectorID);
    }
  });
  currentMap.forEach((currentBinding, connectorID) => {
    if (conflictingConnectorIDs.has(connectorID)) return;
    const submittedBinding = submittedMap.get(connectorID);
    if (!submittedBinding || JSON.stringify(submittedBinding) !== JSON.stringify(currentBinding)) {
      resultMap.set(connectorID, currentBinding);
    }
  });
  return Array.from(resultMap.values()).sort((left, right) =>
    left.connector_id.localeCompare(right.connector_id)
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
    connector_bindings: mergeConnectorBindingDelta(
      submitted.connector_bindings ?? [],
      current.connector_bindings ?? [],
      saved.connector_bindings ?? []
    ),
    connector_reference_commands: (current.connector_reference_commands ?? []).filter(
      command =>
        !(submitted.connector_reference_commands ?? []).some(
          previous => JSON.stringify(previous) === JSON.stringify(command)
        )
    ),
    binding_revision: saved.binding_revision,
  };
}
