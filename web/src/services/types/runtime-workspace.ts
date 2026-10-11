export type RuntimeWorkspaceStatus = 'active' | 'disabled';
export type RuntimeCredentialStatus = 'active' | 'revoked';
export type RuntimeAgentBindingStatus = 'active' | 'disabled';

export interface RuntimeCatalogTag {
  id: string;
  name: string;
}

export interface RuntimeWorkspace {
  runtime_id: string;
  workspace_id: string;
  organization_id: string;
  managed_by_account_id: string;
  status: RuntimeWorkspaceStatus;
  mode: 'attached' | 'isolated';
  policy_json: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimeCredential {
  id: string;
  runtime_workspace_id: string;
  key_prefix: string;
  name: string;
  status: RuntimeCredentialStatus;
  expires_at?: string | null;
  usage_count: number;
  last_used_at?: string | null;
  revoked_at?: string | null;
  created_by_account_id: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimeApp {
  app_key: string;
  agent_id: string;
  workspace_id: string;
  workspace_name: string;
  name: string;
  description?: string;
  icon?: string;
  icon_type?: 'text' | 'image';
  icon_url?: string;
  version: string;
  version_uuid: string;
  published_at: string;
  status: RuntimeAgentBindingStatus;
  supports_stream: boolean;
  file_upload_enabled: boolean;
  supports_vision: boolean;
  agent_memory_enabled: boolean;
  capabilities: string[];
  tags: RuntimeCatalogTag[];
  callable?: boolean;
  unavailable_reason?: string;
}

export interface RuntimeSourceWorkspace {
  id: string;
  name: string;
  current: boolean;
}

export interface RuntimeAgentCandidateSet {
  scope: 'current' | 'authorized';
  cross_workspace_enabled: boolean;
  source_workspaces: RuntimeSourceWorkspace[];
  items: RuntimeAgentCandidate[];
}

export interface RuntimeAgentCandidate {
  agent_id: string;
  workspace_id: string;
  workspace_name: string;
  name: string;
  description?: string;
  icon?: string;
  icon_type?: 'text' | 'image';
  icon_url?: string;
  version: string;
  version_uuid: string;
  published_at: string;
  file_upload_enabled: boolean;
  supports_vision: boolean;
  agent_memory_enabled: boolean;
  capabilities: string[];
  tags: RuntimeCatalogTag[];
}

export interface RuntimeCredentialCreation {
  credential: RuntimeCredential;
  secret: string;
}

export interface RuntimeWorkspaceCredentialInput {
  name?: string;
  expires_at?: string;
  idempotency_key?: string;
}

export interface RuntimeAppCollection {
  revision: number;
  items: RuntimeApp[];
}

export interface RuntimeWorkspaceAgentSelectionInput {
  expected_revision: number;
  agent_ids: string[];
}
