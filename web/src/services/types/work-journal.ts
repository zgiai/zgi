export type WorkJournalJSON =
  | null
  | boolean
  | number
  | string
  | WorkJournalJSON[]
  | { [key: string]: WorkJournalJSON };
export interface WorkJournalRecord {
  id: string;
  kind: string;
  key: string;
  state: string;
  content: WorkJournalJSON;
  sources: WorkJournalJSON[];
  revision: number;
  created_at: string;
  updated_at: string;
}
export type WorkJournalOperationState =
  | 'pending'
  | 'executing'
  | 'succeeded'
  | 'failed'
  | 'unknown';
export interface WorkJournalOperation {
  id: string;
  work_key: string;
  employee_version_id: string;
  connector_id: string;
  connection_id: string;
  operation_id: string;
  input: WorkJournalJSON;
  digest: string;
  state: WorkJournalOperationState;
  attempts: number;
  receipt: WorkJournalJSON;
  evidence_id?: string;
  created_at: string;
  updated_at: string;
}
export interface WorkJournalPage<T> {
  items: T[];
  next_after_id: string | null;
}
export interface WorkJournalHistory {
  items: WorkJournalRecord[];
  next_after_revision: number | null;
}
export interface WorkJournalListParams {
  after_id?: string;
  limit?: number;
}
