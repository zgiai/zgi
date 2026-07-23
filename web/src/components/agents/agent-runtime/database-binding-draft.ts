import type { AgentDatabaseBinding } from '@/services/types/agent';

export function normalizeAgentDatabaseBindings(
  bindings: AgentDatabaseBinding[]
): AgentDatabaseBinding[] {
  const byDataSource = new Map<string, { readable: Set<string>; writable: Set<string> }>();

  bindings.forEach(binding => {
    const dataSourceId = binding.data_source_id.trim();
    if (!dataSourceId) return;

    const readable = new Set(binding.table_ids.map(id => id.trim()).filter(Boolean));
    if (readable.size === 0) return;

    const current = byDataSource.get(dataSourceId) ?? {
      readable: new Set<string>(),
      writable: new Set<string>(),
    };
    readable.forEach(id => current.readable.add(id));
    (binding.writable_table_ids ?? [])
      .map(id => id.trim())
      .filter(id => id && readable.has(id))
      .forEach(id => current.writable.add(id));
    byDataSource.set(dataSourceId, current);
  });

  return Array.from(byDataSource.entries())
    .map(([dataSourceId, tables]) => ({
      data_source_id: dataSourceId,
      table_ids: Array.from(tables.readable).sort(),
      writable_table_ids: Array.from(tables.writable)
        .filter(id => tables.readable.has(id))
        .sort(),
    }))
    .sort((left, right) => left.data_source_id.localeCompare(right.data_source_id));
}

export function planAgentDatabaseSelection(
  bindings: AgentDatabaseBinding[],
  selectedDataSourceIds: string[]
): {
  initialBindings: AgentDatabaseBinding[];
  newDataSourceIds: string[];
} {
  const normalizedBindings = normalizeAgentDatabaseBindings(bindings);
  const selectedIds = Array.from(
    new Set(selectedDataSourceIds.map(id => id.trim()).filter(Boolean))
  );
  const selectedSet = new Set(selectedIds);
  const existingSet = new Set(normalizedBindings.map(binding => binding.data_source_id));

  return {
    initialBindings: normalizedBindings.filter(binding => selectedSet.has(binding.data_source_id)),
    newDataSourceIds: selectedIds.filter(dataSourceId => !existingSet.has(dataSourceId)),
  };
}
