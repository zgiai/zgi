/**
 * Centralized query keys for all domains.
 * Using these constants ensures cache consistency across different hooks.
 */

import type { GetWorkspaceQuotasParams } from '@/services/types/workspace-quota';

// 1. Organization Related
export const ORGANIZATION_KEYS = {
  all: ['organization'] as const,
  lists: () => [...ORGANIZATION_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...ORGANIZATION_KEYS.lists(), params] as const,
  current: () => [...ORGANIZATION_KEYS.all, 'current'] as const,
  currentMembers: (params?: unknown) =>
    [...ORGANIZATION_KEYS.all, 'current-members', params].filter(Boolean),
  roles: (orgId: string) => [...ORGANIZATION_KEYS.all, 'roles', orgId] as const,
  roleDetail: (orgId: string, roleId: string) =>
    [...ORGANIZATION_KEYS.all, 'role-detail', orgId, roleId] as const,
  roleMembers: (orgId: string, roleId: string) =>
    [...ORGANIZATION_KEYS.all, 'role-members', orgId, roleId] as const,
  departments: (orgId: string) => [...ORGANIZATION_KEYS.all, 'departments', orgId] as const,
  departmentMembers: (orgId: string, departmentId?: string, params?: unknown) =>
    ['organization', 'department-members', orgId, departmentId, params].filter(Boolean),
  joinRequests: (orgId: string, params?: unknown) =>
    ['organization', 'join-requests', orgId, params].filter(Boolean),
  inviteLink: (orgId: string, departmentId?: string) =>
    ['organization', 'invite-link', orgId, departmentId].filter(Boolean),
} as const;

// 2. Workspace Related
export const WORKSPACE_KEYS = {
  all: ['workspace'] as const,
  forSwitcher: (orgId: string | null, params?: unknown) =>
    ['workspace', 'for-switcher', orgId, params].filter(Boolean),
  list: (orgId: string | null, keyword: string, page: number, limit: number) =>
    [...WORKSPACE_KEYS.all, 'list', orgId, keyword, page, limit] as const,
  managed: (orgId: string) => [...WORKSPACE_KEYS.all, 'managed', orgId] as const,
  detail: (orgId: string | null, wsId: string | null) =>
    [...WORKSPACE_KEYS.all, 'detail', orgId, wsId] as const,
  members: (orgId: string | null, wsId: string | null, params?: unknown) =>
    [...WORKSPACE_KEYS.all, 'members', orgId, wsId, params].filter(Boolean),
  memberDetail: (orgId: string | null, wsId: string | null, memberId: string | null) =>
    [...WORKSPACE_KEYS.all, 'member-detail', orgId, wsId, memberId].filter(Boolean),
  availableMembers: (orgId: string | null, wsId: string | null, params?: unknown) =>
    [...WORKSPACE_KEYS.all, 'available-members', orgId, wsId, params].filter(Boolean),
  membersInfinite: (orgId: string | null, wsId: string | null, params: unknown) =>
    [...WORKSPACE_KEYS.all, 'members-infinite', orgId, wsId, params].filter(Boolean),
  stats: (wsId: string) => [...WORKSPACE_KEYS.all, 'stats', wsId] as const,
  permissions: (orgId: string | null, wsId: string | null, accountId: string | null) =>
    [...WORKSPACE_KEYS.all, 'permissions', orgId, wsId, accountId] as const,
} as const;

// 3. Agent Related
export const AGENT_KEYS = {
  all: ['agents'] as const,
  lists: () => [...AGENT_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...AGENT_KEYS.lists(), params] as const,
  details: () => [...AGENT_KEYS.all, 'detail'] as const,
  detail: (id: string) => [...AGENT_KEYS.details(), id] as const,
  config: (id: string) => [...AGENT_KEYS.detail(id), 'config'] as const,
  workflowBindingCandidates: (id: string) =>
    [...AGENT_KEYS.detail(id), 'workflow-binding-candidates'] as const,
  runnable: (workspaceId?: string | null) =>
    [...AGENT_KEYS.all, 'runnable-webapps', workspaceId || 'all'] as const,
  runtimeRuns: (agentId: string, params: unknown) =>
    [...AGENT_KEYS.detail(agentId), 'runtime-runs', params] as const,
  runtimeRunDetail: (agentId: string, messageId: string) =>
    [...AGENT_KEYS.detail(agentId), 'runtime-run-detail', messageId] as const,
  runtimeRunSteps: (agentId: string, messageId: string) =>
    [...AGENT_KEYS.detail(agentId), 'runtime-run-steps', messageId] as const,
} as const;

export const PROMPT_KEYS = {
  all: ['prompts'] as const,
  lists: () => [...PROMPT_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...PROMPT_KEYS.lists(), params] as const,
  details: () => [...PROMPT_KEYS.all, 'detail'] as const,
  detail: (id: string) => [...PROMPT_KEYS.details(), id] as const,
  usage: (id: string) => [...PROMPT_KEYS.all, 'usage', id] as const,
  optimizationRuns: (promptId: string, params?: unknown) =>
    params === undefined
      ? ([...PROMPT_KEYS.all, 'optimization-runs', promptId] as const)
      : ([...PROMPT_KEYS.all, 'optimization-runs', promptId, params] as const),
} as const;

// 4. Dataset & Document Related
export const DATASET_KEYS = {
  all: ['datasets'] as const,
  lists: () => [...DATASET_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...DATASET_KEYS.lists(), params] as const,
  details: () => [...DATASET_KEYS.all, 'detail'] as const,
  detail: (id: string) => [...DATASET_KEYS.details(), id] as const,
  // Documents are logically children of datasets
  documents: (datasetId: string) => [...DATASET_KEYS.all, 'documents', datasetId] as const,
  documentList: (datasetId: string, params: unknown) =>
    [...DATASET_KEYS.documents(datasetId), params] as const,
  extractionStrategies: () => [...DATASET_KEYS.all, 'extraction-strategies'] as const,
  documentDetails: (datasetId: string) => [...DATASET_KEYS.all, 'document', datasetId] as const,
  documentDetail: (datasetId: string, documentId: string) =>
    [...DATASET_KEYS.documentDetails(datasetId), documentId] as const,
  processRule: (datasetId: string | null, documentId: string | null) =>
    ['datasets', 'process-rule', datasetId, documentId].filter(Boolean),
  indexingStatus: (datasetId: string, documentId: string) =>
    [...DATASET_KEYS.all, 'indexing-status', datasetId, documentId] as const,
  segments: (datasetId: string, documentId: string) =>
    [...DATASET_KEYS.all, 'segments', datasetId, documentId] as const,
  segmentList: (datasetId: string, documentId: string, params: unknown) =>
    [...DATASET_KEYS.segments(datasetId, documentId), params] as const,
  childSegments: (datasetId: string, documentId: string, segmentId: string) =>
    [...DATASET_KEYS.segments(datasetId, documentId), 'child', segmentId] as const,
  childSegmentList: (datasetId: string, documentId: string, segmentId: string, params: unknown) =>
    [...DATASET_KEYS.childSegments(datasetId, documentId, segmentId), params] as const,
  hitTesting: (datasetId: string) => [...DATASET_KEYS.all, 'hit-testing', datasetId] as const,
  hitTestingHistory: (datasetId: string, params: unknown) =>
    [...DATASET_KEYS.hitTesting(datasetId), 'history', params] as const,
  preview: (datasetKey: string, params: unknown) =>
    [...DATASET_KEYS.all, 'preview', datasetKey, params] as const,
  randomQuestions: (datasetId: string, params: unknown) =>
    [...DATASET_KEYS.all, 'random-questions', datasetId, params] as const,
  graph: (datasetId: string) => [...DATASET_KEYS.all, 'graph', datasetId] as const,
  // Segment questions
  segmentQuestions: (datasetId: string, documentId: string, segmentId: string) =>
    [...DATASET_KEYS.segments(datasetId, documentId), 'questions', segmentId] as const,
} as const;

// 5. Workflow Related
export const WORKFLOW_KEYS = {
  all: ['workflow'] as const,
  drafts: () => [...WORKFLOW_KEYS.all, 'draft'] as const,
  draft: (agentId: string) => [...WORKFLOW_KEYS.drafts(), agentId] as const,
  versions: () => [...WORKFLOW_KEYS.all, 'version'] as const,
  latestVersion: (agentId: string) => [...WORKFLOW_KEYS.versions(), 'latest', agentId] as const,
  runs: (agentId: string) => ['workflow-runs', agentId] as const,
  runList: (agentId: string, params: unknown) =>
    [...WORKFLOW_KEYS.runs(agentId), 'list', params] as const,
  runDetails: () => ['workflow-run-detail'] as const,
  runDetail: (agentId: string, runId: string) =>
    [...WORKFLOW_KEYS.runDetails(), agentId, runId] as const,
  executions: () => ['workflow-run-node-executions'] as const,
  nodeExecutions: (runId: string) => [...WORKFLOW_KEYS.executions(), runId] as const,
  chatMessages: (agentId: string, conversationId: string, params?: unknown) =>
    [...WORKFLOW_KEYS.runs(agentId), 'chat-messages', conversationId, params] as const,
  builtIn: () => ['built-in-workflows'] as const,
} as const;

export const WORKFLOW_TEST_KEYS = {
  all: ['workflow-tests'] as const,
  settings: (agentId: string) => [...WORKFLOW_TEST_KEYS.all, agentId, 'settings'] as const,
  scenarios: (agentId: string) => [...WORKFLOW_TEST_KEYS.all, agentId, 'scenarios'] as const,
  cases: (agentId: string, params?: unknown) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'cases', params] as const,
  generationTaskActive: (agentId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'generation-task-active'] as const,
  generationTaskLatest: (agentId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'generation-task-latest'] as const,
  generationTask: (agentId: string, taskId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'generation-task', taskId] as const,
  scenarioRecognitionTaskActive: (agentId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'scenario-recognition-task-active'] as const,
  scenarioRecognitionTaskLatest: (agentId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'scenario-recognition-task-latest'] as const,
  scenarioRecognitionTask: (agentId: string, taskId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'scenario-recognition-task', taskId] as const,
  batches: (agentId: string) => [...WORKFLOW_TEST_KEYS.all, agentId, 'batches'] as const,
  batchItems: (agentId: string, batchId: string) =>
    [...WORKFLOW_TEST_KEYS.all, agentId, 'batches', batchId, 'items'] as const,
} as const;

export const AICHAT_KEYS = {
  all: ['aichat'] as const,
  skills: () => [...AICHAT_KEYS.all, 'skills'] as const,
  skill: (id: string) => [...AICHAT_KEYS.skills(), id] as const,
  skillConfig: () => [...AICHAT_KEYS.skills(), 'config'] as const,
  skillPreference: () => [...AICHAT_KEYS.skills(), 'preference', 'me'] as const,
  assetOperationAudits: (conversationId: string, params?: unknown) =>
    [...AICHAT_KEYS.all, 'conversations', conversationId, 'asset-operation-audits', params] as const,
  agentSkillVariables: (agentId: string, skillId: string) =>
    [...AICHAT_KEYS.all, 'agents', agentId, 'skills', skillId, 'variables'] as const,
  search: (query: string, limit: number, surface?: string) =>
    [...AICHAT_KEYS.all, 'search', surface ?? 'all', query.trim(), limit] as const,
} as const;

export const MEMORY_KEYS = {
  all: ['memory'] as const,
  me: () => [...MEMORY_KEYS.all, 'me'] as const,
} as const;

// 6. DB Related
export const DB_KEYS = {
  all: ['dbs'] as const,
  lists: () => [...DB_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...DB_KEYS.lists(), params] as const,
  details: () => [...DB_KEYS.all, 'detail'] as const,
  detail: (id: string) => [...DB_KEYS.details(), id] as const,
  tables: (dbId: string) => [...DB_KEYS.all, 'tables', dbId] as const,
  tableList: (dbId: string, params: unknown) => [...DB_KEYS.tables(dbId), 'list', params] as const,
  tableDetail: (dbId: string, id: string) => [...DB_KEYS.tables(dbId), 'detail', id] as const,
  tableRecords: (dbId: string, tableId: string, params: unknown) =>
    [...DB_KEYS.tables(dbId), 'records', tableId, params] as const,
  tableColumns: (dbId: string, tableId: string, includeSystem: boolean) =>
    [
      ...DB_KEYS.tables(dbId),
      'columns',
      tableId,
      includeSystem ? 'with-system' : 'no-system',
    ] as const,
  tablePrompt: (dbId: string, tableId: string) =>
    [...DB_KEYS.tables(dbId), 'prompt', tableId] as const,
  analyzeFile: (datasetId: string, fileId: string) =>
    ['dbs', 'analyze-file', datasetId, fileId] as const,
  sqlOperations: (dbId: string, params: unknown) =>
    [...DB_KEYS.all, 'sql-operations', dbId, params] as const,
  excelImportJob: (dbId: string, jobId: string) =>
    [...DB_KEYS.all, 'excel-import', dbId, 'job', jobId] as const,
  excelImportErrors: (dbId: string, jobId: string, params: unknown) =>
    [...DB_KEYS.excelImportJob(dbId, jobId), 'errors', params] as const,
} as const;

// 7. API Key Related
export const APIKEY_KEYS = {
  all: ['apikey'] as const,
  lists: () => [...APIKEY_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...APIKEY_KEYS.lists(), params] as const,
  detail: (id: string) => [...APIKEY_KEYS.all, 'detail', id] as const,
} as const;

// 8. Provider & Model Related
export const PROVIDER_KEYS = {
  all: ['providers'] as const,
  list: (params: unknown) => [...PROVIDER_KEYS.all, params] as const,
  detail: (provider: string) => ['provider', provider] as const,
  availableCounts: (providers: string[]) =>
    [...PROVIDER_KEYS.all, 'available-counts', providers] as const,
} as const;

export const MODEL_KEYS = {
  allRoot: ['models'] as const,
  all: (params: unknown) => [...MODEL_KEYS.allRoot, 'all', params] as const,
  allInfinite: (params: unknown) => [...MODEL_KEYS.allRoot, 'all-infinite', params] as const,
  available: (params: unknown) => [...MODEL_KEYS.allRoot, 'available', params] as const,
  defaultModels: () => [...MODEL_KEYS.allRoot, 'default-models'] as const,
  defaultModel: (useCase: string) => [...MODEL_KEYS.defaultModels(), useCase] as const,
  list: (params: unknown) => [...MODEL_KEYS.allRoot, params] as const,
  byType: (type: string) => [...MODEL_KEYS.allRoot, 'type', type] as const,
} as const;

export const MODEL_META_KEYS = {
  all: ['modelmeta'] as const,
  status: () => [...MODEL_META_KEYS.all, 'status'] as const,
  providerDiff: () => [...MODEL_META_KEYS.all, 'provider-diff'] as const,
  modelDiff: (provider: string) => [...MODEL_META_KEYS.all, 'model-diff', provider] as const,
  modelUpdateProviders: () => [...MODEL_META_KEYS.all, 'model-update-providers'] as const,
} as const;

// 9. Statistics Related
export const STATS_KEYS = {
  all: ['statistics'] as const,
  usage: (params: unknown) => [...STATS_KEYS.all, 'usage', params] as const,
  userStats: (params: unknown) => [...STATS_KEYS.all, 'user', params] as const,
} as const;

// 10. User & Profile
export const PROFILE_KEYS = {
  all: ['account'] as const,
  current: () => [...PROFILE_KEYS.all, 'profile'] as const,
} as const;

// 11. Webapp Related
export const WEBAPP_KEYS = {
  all: ['webapp'] as const,
  config: (versionUuid: string) => [...WEBAPP_KEYS.all, 'config', versionUuid] as const,
  conversations: (versionUuid: string) =>
    [...WEBAPP_KEYS.all, 'conversations', versionUuid] as const,
  conversationList: (versionUuid: string, params: unknown) =>
    [...WEBAPP_KEYS.conversations(versionUuid), params] as const,
  conversation: (versionUuid: string, conversationId: string) =>
    [...WEBAPP_KEYS.all, 'conversation', versionUuid, conversationId] as const,
} as const;

// 12. Plugin & Marketplace
export const PLUGIN_KEYS = {
  all: ['plugins'] as const,
  marketplace: () => [...PLUGIN_KEYS.all, 'marketplace'] as const,
  marketplaceList: (params: unknown) => [...PLUGIN_KEYS.marketplace(), 'list', params] as const,
  marketplaceCategories: (params: unknown) =>
    [...PLUGIN_KEYS.marketplace(), 'categories', params] as const,
  marketplaceBranding: () => [...PLUGIN_KEYS.marketplace(), 'branding'] as const,
  marketplaceDetail: (id: string) => [...PLUGIN_KEYS.marketplace(), 'detail', id] as const,
  marketplaceVersions: (pluginId: string, params: unknown) =>
    [...PLUGIN_KEYS.marketplace(), 'versions', pluginId, params] as const,
  marketplaceFavorite: (pluginId: string, submitterId: string) =>
    [...PLUGIN_KEYS.marketplace(), 'favorite', pluginId, submitterId] as const,
  installationStatus: (versionId: string) =>
    [...PLUGIN_KEYS.all, 'installation-status', versionId] as const,
} as const;

// 13. System Related
export const SYSTEM_KEYS = {
  all: ['system'] as const,
  settings: () => [...SYSTEM_KEYS.all, 'settings'] as const,
  settingCategories: () => [...SYSTEM_KEYS.settings(), 'categories'] as const,
  settingCategory: (category: string) => [...SYSTEM_KEYS.settings(), 'category', category] as const,
  features: () => [...SYSTEM_KEYS.all, 'features'] as const,
} as const;

// 14. Jobs & Tasks
export const JOB_KEYS = {
  all: ['jobs'] as const,
  importStatus: (jobId: string) => [...JOB_KEYS.all, 'import-status', jobId] as const,
} as const;

// 15. Console/Protocol Related
export const CONSOLE_KEYS = {
  all: ['console'] as const,
  protocols: (params: unknown) => [...CONSOLE_KEYS.all, 'protocols', params] as const,
} as const;

// 16. Channel Related
export const CHANNEL_KEYS = {
  all: ['channels'] as const,
  detail: (id: string) => ['channel', id] as const,
  list: (params: unknown) => [...CHANNEL_KEYS.all, params] as const,
} as const;

// 17. Dashboard Related
export const DASHBOARD_KEYS = {
  all: ['dashboard'] as const,
  stats: () => [...DASHBOARD_KEYS.all, 'stats'] as const,
} as const;

// 18. Payment Related
export const PAY_KEYS = {
  aiCredits: () => ['ai-credits', 'me'] as const,
  wallet: () => ['wallet', 'me'] as const,
  bills: (params: unknown) => ['bills', params] as const,
  monthlyStats: () => ['monthly-stats', 'me'] as const,
  products: () => ['ai-credit-products'] as const,
} as const;

// 19. File Related
export const FILE_KEYS = {
  all: ['files'] as const,
  preview: (fileId?: string) => [...FILE_KEYS.all, 'preview', fileId || ''] as const,
  originalPreviewUrl: (fileId?: string) =>
    [...FILE_KEYS.all, 'original-preview-url', fileId || ''] as const,
} as const;

// 20. Workspace Quota Related
export const WORKSPACE_QUOTA_KEYS = {
  all: ['workspace-quotas'] as const,
  lists: () => [...WORKSPACE_QUOTA_KEYS.all, 'list'] as const,
  list: (params: GetWorkspaceQuotasParams | undefined) =>
    [...WORKSPACE_QUOTA_KEYS.lists(), params] as const,
  detail: (workspaceId: string) => [...WORKSPACE_QUOTA_KEYS.all, 'detail', workspaceId] as const,
} as const;

// 21. Automation Related
export const AUTOMATION_KEYS = {
  all: ['automation'] as const,
  lists: () => [...AUTOMATION_KEYS.all, 'list'] as const,
  list: (params: unknown) => [...AUTOMATION_KEYS.lists(), params] as const,
  details: () => [...AUTOMATION_KEYS.all, 'detail'] as const,
  detailPrefix: (taskId: string) => [...AUTOMATION_KEYS.details(), taskId] as const,
  detail: (taskId: string, params: unknown) =>
    [...AUTOMATION_KEYS.detailPrefix(taskId), params] as const,
  runs: (taskId: string) => [...AUTOMATION_KEYS.all, 'runs', taskId] as const,
  runList: (taskId: string, params: unknown) => [...AUTOMATION_KEYS.runs(taskId), params] as const,
} as const;
