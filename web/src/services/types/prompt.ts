export type PromptSource = 'official' | 'workspace' | 'personal';
export type PromptType = 'text' | 'chat';
export type PromptOptimizerGoal = 'general' | 'reliable' | 'structured' | 'deep';
export type PromptOptimizerVariant = 'safe' | 'balanced' | 'advanced';

export interface PromptPlaygroundMessage {
  role: 'system' | 'user' | 'assistant';
  content: string;
}

export interface PromptVersionPayload {
  prompt_type: PromptType;
  content: string | Array<{ role: 'system' | 'user' | 'assistant'; content: string }>;
  config?: Record<string, unknown>;
  labels?: string[];
  commit_message?: string | null;
}

export interface CreatePromptRequest {
  workspace_id: string;
  source: Exclude<PromptSource, 'official'>;
  name: string;
  slug?: string;
  description?: string | null;
  locale?: string;
  category?: string | null;
  tags?: string[];
  initial_version: PromptVersionPayload;
}

export interface UpdatePromptRequest {
  name?: string;
  description?: string | null;
  locale?: string;
  category?: string | null;
  tags?: string[];
  source?: Exclude<PromptSource, 'official'>;
}

export interface SetPromptLabelsRequest {
  version: number;
  labels: string[];
}

export interface PromptVersion {
  id: string;
  version: number;
  prompt_type: PromptType;
  content: string | Array<{ role: 'system' | 'user' | 'assistant'; content: string }>;
  config: Record<string, unknown>;
  labels: string[];
  commit_message?: string | null;
  created_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface PromptSummary {
  id: string;
  workspace_id?: string | null;
  owner_account_id?: string | null;
  source: PromptSource;
  name: string;
  slug: string;
  description?: string | null;
  locale: string;
  category?: string | null;
  tags: string[];
  latest_version: number;
  latest_labels: string[];
  latest_prompt_type: PromptType;
  is_owned: boolean;
  created_at: string;
  updated_at: string;
}

export interface PromptDetail extends PromptSummary {
  versions: PromptVersion[];
}

export interface PromptPickerSelection {
  prompt: PromptSummary;
  version: PromptVersion;
  reference:
    | {
        mode: 'label';
        label: string;
      }
    | {
        mode: 'version';
        version: number;
      };
}

export interface PromptListParams {
  page?: number;
  limit?: number;
  keyword?: string;
  workspace_id?: string;
  locale?: string;
  source?: PromptSource;
  category?: string;
}

export interface PromptList {
  data: PromptSummary[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

export interface PromptOptimizeRequest {
  raw_prompt: string;
  goal?: PromptOptimizerGoal;
  preserve_variables?: boolean;
  provider?: string;
  model?: string;
  prompt_id?: string;
}

export interface PromptOptimizeResult {
  goal: PromptOptimizerGoal;
  preserve_variables: boolean;
  detected_variables: string[];
  run_id: string;
  output: string;
  variants: Record<PromptOptimizerVariant, string>;
}

export interface PromptPlaygroundRequest {
  prompt: string;
  messages?: PromptPlaygroundMessage[];
  input?: string;
  variables?: Record<string, string>;
  provider?: string;
  model?: string;
}

export interface PromptUsageReference {
  agent_id: string;
  agent_name: string;
  workflow_id: string;
  node_id: string;
  node_title: string;
  reference_mode?: string;
  label?: string | null;
  version?: number | null;
  updated_at: string;
}

export interface PromptUsageRecentRun {
  workflow_run_id?: string | null;
  agent_id: string;
  agent_name: string;
  node_id: string;
  node_title: string;
  status: string;
  prompt_version?: number | null;
  requested_label?: string | null;
  requested_version?: number | null;
  created_at: string;
  finished_at?: string | null;
  elapsed_time: number;
}

export interface PromptUsageSummary {
  linked_agents_count: number;
  linked_nodes_count: number;
  total_run_count: number;
  last_run_at?: string | null;
  version_metrics: Array<{
    version: number;
    run_count: number;
    last_run_at?: string | null;
  }>;
  label_metrics: Array<{
    label: string;
    run_count: number;
    last_run_at?: string | null;
  }>;
  references: PromptUsageReference[];
  recent_runs: PromptUsageRecentRun[];
}

export interface PromptOptimizationRun {
  id: string;
  prompt_id?: string | null;
  goal: PromptOptimizerGoal;
  provider?: string | null;
  model?: string | null;
  preserve_variables: boolean;
  detected_variables: string[];
  raw_prompt: string;
  output: string;
  variants: Record<PromptOptimizerVariant, string>;
  adopted_variant?: PromptOptimizerVariant | null;
  adopted_prompt_version_id?: string | null;
  adopted_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface PromptOptimizationRunList {
  data: PromptOptimizationRun[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

export interface PromptOptimizationRunListParams {
  page?: number;
  limit?: number;
}

export interface AdoptPromptOptimizationRunRequest {
  variant: PromptOptimizerVariant;
  commit_message?: string | null;
}
