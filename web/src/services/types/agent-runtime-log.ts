export interface AgentRuntimeRunsQuery {
  page?: number;
  limit?: number;
  triggered_from?: 'web-app' | string;
  source?: 'webapp' | 'console' | 'external-api' | string;
  q?: string;
  conversation_id?: string;
}

export interface AgentRuntimeRunItem {
  id: string;
  conversation_id: string;
  status: string;
  query: string;
  answer_preview?: string;
  model_name?: string;
  model_provider?: string | null;
  elapsed_time?: number;
  total_tokens?: number;
  total_steps?: number;
  created_at?: number;
  finished_at?: number | null;
  error?: string;
  source?: string;
  source_web_app_id?: string | null;
}

export interface AgentRuntimeRunsList {
  page: number;
  limit: number;
  total?: number;
  has_more: boolean;
  data: AgentRuntimeRunItem[];
}

export interface AgentRuntimeRunDetail {
  id: string;
  conversation_id: string;
  status: string;
  query: string;
  answer: string;
  model_name?: string;
  model_provider?: string | null;
  model_parameters?: Record<string, unknown>;
  usage?: unknown;
  elapsed_time?: number;
  total_tokens?: number;
  total_steps?: number;
  created_at?: number;
  finished_at?: number | null;
  error?: string;
  source?: string;
  source_web_app_id?: string | null;
}

export type AgentRuntimeStepType =
  | 'user_input'
  | 'skill'
  | 'tool'
  | 'tool_call'
  | 'skill_load'
  | 'reference_read'
  | 'intermediate_answer'
  | 'final_answer'
  | 'user_input_request'
  | 'guardrail'
  | 'workflow_run'
  | 'workflow_node'
  | 'workflow_approval'
  | 'model_call'
  | 'model_answer'
  | 'agent_step'
  | string;

export interface AgentRuntimeStep {
  id: string;
  index: number;
  type: AgentRuntimeStepType;
  title: string;
  status: string;
  input?: unknown;
  output?: unknown;
  process?: Record<string, unknown> | null;
  elapsed_time?: number;
  created_at?: number | null;
  finished_at?: number | null;
  error?: string;
}
