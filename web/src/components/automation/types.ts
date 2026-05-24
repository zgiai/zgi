import type {
  AutomationBodyType,
  AutomationWorkflowVersionStrategy,
  AutomationTask,
  AutomationTaskAction,
  AutomationTaskDetailData,
  AutomationTaskStatus,
  CreateAutomationTaskRequest,
  UpdateAutomationTaskRequest,
} from '@/services/types/automation';

export type TaskPanelMode = 'create' | 'edit' | null;

export type TaskPanelView = 'view' | 'create' | 'edit';

export type TaskPanelTab = 'overview' | 'runs';

export type TaskStatusFilterKey = 'all' | 'active' | 'paused' | 'completed' | 'archived';

export type TaskWeekdayKey = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun';

export type TaskRecurringMode = 'daily' | 'weekly' | 'customCron';

export interface TaskDraftSupportState {
  hasUnknownSchedule: boolean;
  hasUnknownAction: boolean;
  hasUnknownChannel: boolean;
  isEditable: boolean;
}

export interface TaskDraftSchedule {
  scheduleType: 'once' | 'cron';
  timezone: string;
  onceRunAt: string;
  recurringMode: TaskRecurringMode;
  recurringTime: string;
  recurringDays: TaskWeekdayKey[];
  cronExpr: string;
  rawConfig: Record<string, unknown> | null;
}

export interface TaskDraftAction {
  clientId: string;
  actionType: string;
  channelType: string;
  enabled: boolean;
  recipients: string[];
  subject: string;
  bodyType: AutomationBodyType;
  body: string;
  smsTemplate: string;
  smsTemplateParams: Record<string, string>;
  workflowAgentId: string;
  workflowVersionStrategy: AutomationWorkflowVersionStrategy;
  workflowVersionUuid: string;
  workflowInputsJson: string;
  workflowTimeoutSeconds: string;
  rawConfig: Record<string, unknown> | null;
}

export interface TaskDraftActionErrors {
  actionType?: string;
  channelType?: string;
  recipients?: string;
  subject?: string;
  bodyType?: string;
  body?: string;
  smsTemplate?: string;
  smsTemplateParams?: Record<string, string | undefined>;
  workflowAgentId?: string;
  workflowVersionUuid?: string;
  workflowInputsJson?: string;
  workflowTimeoutSeconds?: string;
}

export interface TaskDraft {
  name: string;
  description: string;
  schedule: TaskDraftSchedule;
  actions: TaskDraftAction[];
  support: TaskDraftSupportState;
}

export interface TaskValidationErrors {
  name?: string;
  onceRunAt?: string;
  timezone?: string;
  recurringTime?: string;
  recurringDays?: string;
  cronExpr?: string;
  actionsRequired?: string;
  actionErrors?: Record<string, TaskDraftActionErrors>;
}

export interface TaskRouteState {
  taskId: string | null;
  mode: TaskPanelMode;
  tab: TaskPanelTab;
  page: number;
  statusQuery: string;
  selectedStatuses: AutomationTaskStatus[];
  filterKey: TaskStatusFilterKey;
  panelView: TaskPanelView | null;
  panelOpen: boolean;
}

export type TaskMutationRequest = CreateAutomationTaskRequest | UpdateAutomationTaskRequest;

export interface TaskScheduleSummary {
  title: string;
  description: string;
  badges: string[];
}

export interface TaskDetailViewData extends AutomationTaskDetailData {
  task: AutomationTask;
  actions: AutomationTaskAction[];
}

export interface TaskWorkflowOption {
  id: string;
  name: string;
  description?: string | null;
  isPublished: boolean;
}

export interface TaskWorkflowVersionOption {
  id: string;
  workflowId: string;
  label: string;
  version: string;
  createdAt?: string;
}
