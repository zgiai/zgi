// Memory history (ADR 0013): each change to a user's memory, with the content
// before and after, the user's words it was based on, and why it was made.

export interface MemoryHistoryVersion {
  id: string;
  key: string;
  definition_id?: string;
  action: 'create' | 'update' | 'delete' | 'clear' | 'undo';
  before_revision?: number;
  after_revision?: number;
  before: string;
  after: string;
  evidence?: string;
  reason?: string;
  content_purged: boolean;
  revertible: boolean;
}

export interface MemoryHistoryCommit {
  id: string;
  source_kind: string;
  actor_type: string;
  source: string;
  conversation_id?: string;
  message_id?: string;
  created_at: string;
  versions: MemoryHistoryVersion[];
}

// Memory archive (ADR 0014): older memory moved out of full files.
export interface MemoryArchiveEntry {
  id: string;
  content: string;
  tags?: string;
  source_key?: string;
  created_at: string;
  updated_at?: string;
}

// A DigitalHuman's note on the user (ADR 0014 /user/notes/), read-only.
export interface MemoryNote {
  path: string;
  tags?: string;
  content: string;
  updated_at: string;
}

export interface MemoryHistoryPage {
  commits: MemoryHistoryCommit[];
  next_before?: string;
}
