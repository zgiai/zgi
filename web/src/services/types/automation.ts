// Automation task module types

import type { PaginationParams } from './common';

export type AutomationTaskStatus = 'draft' | 'active' | 'paused' | 'completed' | 'archived';

export type AutomationScheduleType = 'once' | 'cron';

export type AutomationActionType = 'send_notification' | 'run_workflow';

export type AutomationChannelType = 'email' | 'sms';

export type AutomationBodyType = 'text/plain' | 'text/html';

export type AutomationRunStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';

export type AutomationTriggerSource = 'scheduler' | 'manual_run' | 'retry';

export type AutomationWorkflowVersionStrategy = 'latest_published' | 'pinned';

export interface AutomationOnceScheduleConfig {
  run_at: string | number;
}

export interface AutomationCronScheduleConfig {
  cron_expr: string;
}

export type AutomationScheduleConfig = AutomationOnceScheduleConfig | AutomationCronScheduleConfig;

export interface AutomationEmailNotificationActionConfig {
  channel_type: 'email';
  to: string[];
  subject: string;
  body: string;
  body_type: AutomationBodyType;
}

export interface AutomationSMSNotificationActionConfig {
  channel_type: 'sms';
  to: string[];
  template: 'pending_action_notification';
  template_params: {
    notification_title: string;
    link_suffix: string;
  };
}

export type AutomationNotificationActionConfig =
  | AutomationEmailNotificationActionConfig
  | AutomationSMSNotificationActionConfig;

export interface AutomationRunWorkflowActionConfig {
  workflow_ref: {
    agent_id: string;
    workflow_id?: string;
    version_strategy: AutomationWorkflowVersionStrategy;
    version_uuid?: string;
  };
  inputs?: Record<string, unknown>;
  execution?: {
    timeout_seconds?: number;
  };
}

export type AutomationActionConfig =
  | AutomationNotificationActionConfig
  | AutomationRunWorkflowActionConfig;

interface AutomationTaskActionInputBase {
  action_order?: number;
  enabled?: boolean;
}

export interface AutomationNotificationTaskActionInput extends AutomationTaskActionInputBase {
  action_type: 'send_notification';
  config: AutomationNotificationActionConfig;
}

export interface AutomationRunWorkflowTaskActionInput extends AutomationTaskActionInputBase {
  action_type: 'run_workflow';
  config: AutomationRunWorkflowActionConfig;
}

export type AutomationTaskActionInput =
  | AutomationNotificationTaskActionInput
  | AutomationRunWorkflowTaskActionInput;

export interface AutomationTask {
  id: string;
  organization_id?: string | null;
  workspace_id?: string | null;
  name: string;
  description?: string | null;
  status: AutomationTaskStatus;
  trigger_type?: string | null;
  schedule_type: AutomationScheduleType;
  timezone?: string;
  schedule_config: AutomationScheduleConfig;
  next_run_at?: number | null;
  last_run_at?: number | null;
  last_run_status?: AutomationRunStatus | null;
  source_type?: string | null;
  source_ref?: string | null;
  source_snapshot?: Record<string, unknown> | null;
  created_by?: string | null;
  updated_by?: string | null;
  created_at?: number;
  updated_at?: number;
}

interface AutomationTaskActionBase {
  id: string;
  task_id: string;
  action_order?: number;
  enabled?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface AutomationNotificationTaskAction extends AutomationTaskActionBase {
  action_type: 'send_notification';
  config: AutomationNotificationActionConfig;
}

export interface AutomationRunWorkflowTaskAction extends AutomationTaskActionBase {
  action_type: 'run_workflow';
  config: AutomationRunWorkflowActionConfig;
}

export type AutomationTaskAction =
  | AutomationNotificationTaskAction
  | AutomationRunWorkflowTaskAction;

export interface AutomationTaskRun {
  id: string;
  task_id: string;
  trigger_source: AutomationTriggerSource;
  scheduled_for?: number | null;
  started_at?: number | null;
  finished_at?: number | null;
  status: AutomationRunStatus;
  runtime_context?: Record<string, unknown> | null;
  error_summary?: string | null;
  created_at?: number;
  updated_at?: number;
}

export interface AutomationTaskActionRun {
  id: string;
  task_run_id: string;
  task_action_id: string;
  action_type: AutomationActionType;
  channel_type?: AutomationChannelType | null;
  request_payload?: AutomationActionConfig | null;
  response_payload?: Record<string, unknown> | null;
  error_message?: string | null;
  status: AutomationRunStatus;
  started_at?: number | null;
  finished_at?: number | null;
  created_at?: number;
  updated_at?: number;
}

export interface AutomationTaskDetailData {
  task: AutomationTask;
  actions: AutomationTaskAction[];
}

export interface AutomationTaskListItem {
  task: AutomationTask;
  actions: AutomationTaskAction[];
}

export interface AutomationTaskList {
  items: AutomationTaskListItem[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

export interface AutomationTaskRunItem {
  run: AutomationTaskRun;
  action_runs: AutomationTaskActionRun[];
}

export interface AutomationTaskRunsList {
  task_id: string;
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
  runs: AutomationTaskRunItem[];
}

export interface AutomationManualRunData {
  task_id: string;
  run: AutomationTaskRun;
}

export interface AutomationOperationResult {
  task_id: string;
  operation: 'pause' | 'resume' | 'archive';
  status: 'ok';
}

export interface CreateAutomationTaskRequest {
  workspace_id?: string;
  name: string;
  description?: string | null;
  schedule_type: AutomationScheduleType;
  timezone?: string;
  schedule_config: AutomationScheduleConfig;
  actions: AutomationTaskActionInput[];
}

export interface UpdateAutomationTaskRequest {
  workspace_id?: string;
  name: string;
  description?: string | null;
  schedule_type: AutomationScheduleType;
  timezone?: string;
  schedule_config: AutomationScheduleConfig;
  actions: AutomationTaskActionInput[];
}

export interface GenerateAutomationTaskDraftRequest {
  workspace_id?: string;
  prompt: string;
  locale?: string;
  timezone?: string;
  provider?: string;
  model?: string;
}

export interface GeneratedAutomationTaskDraft {
  name: string;
  description?: string | null;
  schedule_type: AutomationScheduleType;
  timezone?: string;
  schedule_config: AutomationScheduleConfig;
  actions: AutomationTaskActionInput[];
}

export interface GenerateAutomationTaskDraftResponse {
  draft: GeneratedAutomationTaskDraft;
  missing_fields?: string[];
  warnings?: string[];
  summary?: string;
  provider?: string;
  model?: string;
}

export interface GetAutomationTasksParams extends PaginationParams {
  workspace_id?: string;
  statuses?: string;
}

export interface GetAutomationTaskParams {
  workspace_id?: string;
}

export interface GetAutomationTaskRunsParams extends PaginationParams {
  workspace_id?: string;
}

export interface AutomationTaskActionParams {
  workspace_id?: string;
}
