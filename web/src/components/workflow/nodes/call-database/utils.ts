import type { Db, DbTable, DbTableColumn } from '@/services/types/db';
import type { DatabaseSourceRef, DatabaseType, TableRef } from './config';

const DEFAULT_DB_TYPE: DatabaseType = 'postgres';

export interface TableLookupKey {
  schema: string;
  name: string;
}

export const createDatabaseSourceRef = (db: Db | null | undefined): DatabaseSourceRef | null => {
  if (!db) return null;
  return {
    id: db.id,
    name: db.name,
    type: inferDatabaseType(db),
    schema_name: db.schema_name?.trim() || 'public',
  };
};

export const inferDatabaseType = (db: Db | null | undefined): DatabaseType => {
  if (!db) return DEFAULT_DB_TYPE;
  const provider = (db.provider || '').toLowerCase();
  if (provider.includes('mysql')) return 'mysql';
  if (provider.includes('sqlite')) return 'sqlite';
  if (provider.includes('mssql')) return 'mssql';
  if (provider.includes('oracle')) return 'oracle';
  if (provider.includes('postgres')) return 'postgres';
  return DEFAULT_DB_TYPE;
};

export const deriveTableSchema = (
  table: DbTable | null | undefined,
  fallbackSchema: string = 'public'
): string => {
  if (!table) return fallbackSchema;
  const schemaCandidate = table.schema_name || fallbackSchema;
  if (typeof schemaCandidate === 'string' && schemaCandidate.trim().length > 0) {
    return schemaCandidate.trim();
  }
  if (table.organization_id && table.organization_id.trim().length > 0) {
    return table.organization_id.trim();
  }
  const parts = (table.table_name || '').split('.');
  if (parts.length === 2 && parts[0]) {
    return parts[0];
  }
  return fallbackSchema;
};

// Map to backend table_name (without schema prefix)
export const deriveTableName = (table: DbTable | null | undefined): string => {
  if (!table) return '';
  const tname = (table.table_name || '').trim();
  if (tname.length > 0) {
    const pieces = tname.split('.');
    return pieces.length === 2 ? (pieces[1] ?? '') : tname;
  }
  return '';
};

// Friendly label from DbTable.name for UI display
export const deriveTableLabel = (table: DbTable | null | undefined): string | undefined => {
  if (!table) return undefined;
  const n = (table.name || '').trim();
  return n.length > 0 ? n : undefined;
};

export const buildTableRef = (
  table: DbTable,
  columns: string[] | undefined = undefined,
  fallbackSchema?: string
): TableRef => ({
  schema: deriveTableSchema(table, fallbackSchema),
  name: deriveTableName(table),
  label: deriveTableLabel(table),
  columns,
  id: table.id,
  table_id: table.table_id,
});

export const mergeTableSelection = (current: TableRef[], next: TableRef): TableRef[] => {
  const idx = current.findIndex(item => item.schema === next.schema && item.name === next.name);
  if (idx === -1) {
    return [...current, next];
  }
  const existing = current[idx];
  const merged: TableRef = {
    ...existing,
    ...next,
    columns: next.columns ?? existing.columns,
  };
  const clone = current.slice();
  clone[idx] = merged;
  return clone;
};

export const removeTableSelection = (current: TableRef[], key: TableLookupKey): TableRef[] =>
  current.filter(item => !(item.schema === key.schema && item.name === key.name));

export const extractColumnNames = (columns: DbTableColumn[] | null | undefined): string[] => {
  if (!Array.isArray(columns)) return [];
  return columns
    .map(col => col.name?.trim())
    .filter((name): name is string => Boolean(name && name.length > 0));
};

export type InsertTokenKind = 'database' | 'schema' | 'table' | 'column';

export interface InsertTokenPayload {
  kind: InsertTokenKind;
  value: string;
  table?: TableRef;
}

export const formatQualifiedTable = (table: TableRef): string => `${table.schema}.${table.name}`;

export const buildColumnToken = (_table: TableRef, column: string): string => column;

// Display helper for tables in menus and lists
export const getTableDisplayName = (table: TableRef): string => `${table.schema}.${table.name}`;
