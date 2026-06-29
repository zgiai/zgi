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
}

export type DashboardRecentWorkType = 'conversation' | 'agent' | 'workflow' | 'dataset' | 'database';

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
}

export interface DashboardRecentWork {
  items: DashboardRecentWorkItem[];
}
