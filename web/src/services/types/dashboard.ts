export interface DashboardStats {
  models: {
    total: number;
    by_usecase: {
      embedding?: number;
      'function-calling'?: number;
      'image-gen'?: number;
      moderation?: number;
      'realtime-audio'?: number;
      reasoning?: number;
      rerank?: number;
      'speech-to-text'?: number;
      'text-chat'?: number;
      'text-to-speech'?: number;
      'video-gen'?: number;
      vision?: number;
    };
  };
  resources: {
    workspaces: number;
    agents: number;
    datasets: number;
    data_sources: number;
    files: number;
  };
  activity?: {
    direct_conversations: number;
    created_agents: number;
    created_datasets: number;
  };
}

export interface DashboardStatsParams {
  scope?: DashboardRecentWorkScope;
  workspace_id?: string;
}

export type DashboardRecentWorkType =
  | 'conversation'
  | 'agent'
  | 'workflow'
  | 'dataset'
  | 'database';

export type DashboardRecentWorkScope = 'overview' | 'workspace';

export interface DashboardRecentWorkParams {
  scope?: DashboardRecentWorkScope;
  workspace_id?: string;
  limit?: number;
}

export interface DashboardRecentWorkItem {
  id: string;
  type: DashboardRecentWorkType;
  title: string;
  resource_id: string;
  parent_id?: string;
  workspace_id?: string;
  workspace_name?: string;
  updated_at: number;
  /** Agent and workflow items only: data used to pick a detail route the account may open. */
  agent_type?: string;
  is_published?: boolean;
  can_open_editor?: boolean;
  can_view_logs?: boolean;
}

export interface DashboardRecentWork {
  items: DashboardRecentWorkItem[];
}
