export interface PermissionItem {
  code: PermissionCode;
  name: string;
  description: string;
}

export interface PermissionModule {
  key: string;
  title: string;
  permissions: PermissionItem[];
}

const zhSegmentLabels: Record<string, string> = {
  workspace: '工作空间',
  settings: '设置',
  member: '成员',
  permission: '权限',
  billing: '财务',
  audit: '审计',
  transfer: '转让',
  archive: '归档',
  agent: '智能体',
  runtime: '运行时',
  access: '访问范围',
  config: '配置',
  logs: '日志',
  stats: '统计',
  conversation: '会话',
  workflow: '工作流',
  run: '运行',
  draft: '草稿',
  stop: '停止',
  debug: '调试',
  events: '事件',
  prompt: '提示词',
  version: '版本',
  label: '标签',
  optimize: '优化',
  playground: '调试台',
  usage: '用量',
  knowledge: '知识库',
  base: '知识库',
  folder: '文件夹',
  document: '文档',
  segment: '分段',
  index: '索引',
  graph: '图谱',
  database: '数据库',
  data: '数据',
  edit: '编辑',
  ai: 'AI',
  query: '查询',
  schema: '结构',
  record: '记录',
  import: '导入',
  analyze: '分析',
  execute: '执行',
  errors: '错误',
  read: '读取',
  write: '写入',
  guard: '安全',
  policy: '策略',
  table: '表',
  sql: 'SQL',
  operation: '操作',
  file: '文件',
  upload: '上传',
  create: '创建',
  download: '下载',
  move: '移动',
  metadata: '元数据',
  preview: '预览',
  text: '文本',
  share: '共享',
  favorite: '收藏',
  related: '关联',
  content: '内容',
  parse: '解析',
  pdf: 'PDF',
  render: '渲染',
  shadow: '影子数据',
  chunk: '分块',
  dashboard: '概览',
  recent: '最近工作',
  models: '模型',
  view: '查看',
  manage: '管理',
  update: '更新',
  delete: '删除',
  copy: '复制',
  export: '导出',
  publish: '发布',
  lock: '锁定',
};

const enSegmentLabels: Record<string, string> = {
  workspace: 'Workspace',
  settings: 'Settings',
  member: 'Member',
  permission: 'Permission',
  billing: 'Billing',
  audit: 'Audit',
  transfer: 'Transfer',
  archive: 'Archive',
  agent: 'Agent',
  runtime: 'Runtime',
  access: 'Access',
  config: 'Config',
  logs: 'Logs',
  stats: 'Stats',
  conversation: 'Conversation',
  workflow: 'Workflow',
  run: 'Run',
  draft: 'Draft',
  stop: 'Stop',
  debug: 'Debug',
  events: 'Events',
  prompt: 'Prompt',
  version: 'Version',
  label: 'Label',
  optimize: 'Optimize',
  playground: 'Playground',
  usage: 'Usage',
  knowledge: 'Knowledge',
  base: 'Base',
  folder: 'Folder',
  document: 'Document',
  segment: 'Segment',
  index: 'Index',
  graph: 'Graph',
  database: 'Database',
  data: 'Data',
  edit: 'Edit',
  ai: 'AI',
  query: 'Query',
  schema: 'Schema',
  record: 'Record',
  import: 'Import',
  analyze: 'Analyze',
  execute: 'Execute',
  errors: 'Errors',
  read: 'Read',
  write: 'Write',
  guard: 'Guard',
  policy: 'Policy',
  table: 'Table',
  sql: 'SQL',
  operation: 'Operation',
  file: 'File',
  upload: 'Upload',
  create: 'Create',
  download: 'Download',
  move: 'Move',
  metadata: 'Metadata',
  preview: 'Preview',
  text: 'Text',
  share: 'Share',
  favorite: 'Favorite',
  related: 'Related',
  content: 'Content',
  parse: 'Parse',
  pdf: 'PDF',
  render: 'Render',
  shadow: 'Shadow',
  chunk: 'Chunk',
  dashboard: 'Dashboard',
  recent: 'Recent Work',
  models: 'Models',
  view: 'View',
  manage: 'Manage',
  update: 'Update',
  delete: 'Delete',
  copy: 'Copy',
  export: 'Export',
  publish: 'Publish',
  lock: 'Lock',
};

const isChineseLocale = (locale?: string) => locale?.toLowerCase().startsWith('zh');

const permissionFallbackNames: Record<string, { zhHans: string; enUS: string }> = {
  'agent.create': { zhHans: '新建智能体', enUS: 'Create agents' },
  'agent.update': { zhHans: '编辑智能体', enUS: 'Edit agents' },
  'agent.delete': { zhHans: '删除智能体', enUS: 'Delete agents' },
  'agent.move': { zhHans: '移动智能体', enUS: 'Move agents' },
  'agent.publish': { zhHans: '发布智能体', enUS: 'Publish agents' },
  'agent.runtime_access.manage': { zhHans: '配置智能体访问范围', enUS: 'Configure agent access' },
  'agent.logs.view': { zhHans: '查看智能体运行日志', enUS: 'View agent logs' },
  'workflow.view': { zhHans: '查看工作流', enUS: 'View workflows' },
  'workflow.create': { zhHans: '新建工作流', enUS: 'Create workflows' },
  'workflow.update': { zhHans: '编辑工作流', enUS: 'Edit workflows' },
  'workflow.delete': { zhHans: '删除工作流', enUS: 'Delete workflows' },
  'workflow.move': { zhHans: '移动工作流', enUS: 'Move workflows' },
  'workflow.import': { zhHans: '导入/导出工作流', enUS: 'Import/export workflows' },
  'workflow.run.draft': { zhHans: '调试运行工作流', enUS: 'Run workflow drafts' },
  'workflow.publish': { zhHans: '发布工作流', enUS: 'Publish workflows' },
  'workflow.runtime_access.manage': {
    zhHans: '配置工作流访问范围',
    enUS: 'Configure workflow access',
  },
  'workflow.logs.view': { zhHans: '查看工作流日志', enUS: 'View workflow logs' },
  'knowledge_base.retrieval_test': { zhHans: '使用召回测试', enUS: 'Run retrieval tests' },
  'knowledge_base.folder_manage': { zhHans: '管理知识库文件夹', enUS: 'Manage knowledge folders' },
  'knowledge_base.create': { zhHans: '新建知识库', enUS: 'Create knowledge bases' },
  'knowledge_base.update': { zhHans: '编辑知识库', enUS: 'Edit knowledge bases' },
  'knowledge_base.delete': { zhHans: '删除知识库', enUS: 'Delete knowledge bases' },
  'knowledge_base.move': { zhHans: '移动知识库', enUS: 'Move knowledge bases' },
  'knowledge_base.document.view': { zhHans: '查看知识文档', enUS: 'View knowledge documents' },
  'knowledge_base.document.create': { zhHans: '新增知识文档', enUS: 'Add knowledge documents' },
  'knowledge_base.document.update': { zhHans: '编辑知识文档', enUS: 'Edit knowledge documents' },
  'knowledge_base.document.delete': { zhHans: '删除知识文档', enUS: 'Delete knowledge documents' },
  'knowledge_base.segment.update': { zhHans: '编辑文档分段', enUS: 'Edit document chunks' },
  'knowledge_base.segment.delete': { zhHans: '删除文档分段', enUS: 'Delete document chunks' },
  'knowledge_base.index.manage': { zhHans: '管理知识库索引', enUS: 'Manage knowledge indexes' },
  'knowledge_base.graph.view': { zhHans: '查看知识图谱', enUS: 'View knowledge graph' },
  'knowledge_base.graph.manage': { zhHans: '管理知识图谱', enUS: 'Manage knowledge graph' },
  'database.data_edit': { zhHans: '编辑数据表内容', enUS: 'Edit table data' },
  'database.ai_query': { zhHans: '使用 AI 查询数据', enUS: 'Use AI data query' },
  'database.create': { zhHans: '新建数据库', enUS: 'Create databases' },
  'database.update': { zhHans: '编辑数据库', enUS: 'Edit databases' },
  'database.delete': { zhHans: '删除数据库', enUS: 'Delete databases' },
  'database.move': { zhHans: '移动数据库', enUS: 'Move databases' },
  'database.schema.view': { zhHans: '查看表结构', enUS: 'View table schema' },
  'database.schema.manage': { zhHans: '管理表结构', enUS: 'Manage table schema' },
  'database.record.view': { zhHans: '查看表数据', enUS: 'View table records' },
  'database.record.create': { zhHans: '新增表数据', enUS: 'Add table records' },
  'database.record.update': { zhHans: '修改表数据', enUS: 'Update table records' },
  'database.record.delete': { zhHans: '删除表数据', enUS: 'Delete table records' },
  'database.import.analyze': { zhHans: '分析导入文件', enUS: 'Analyze imports' },
  'database.import.execute': { zhHans: '执行数据导入', enUS: 'Run data imports' },
  'database.ai_query.read': { zhHans: 'AI 查询读取数据', enUS: 'AI query read access' },
  'database.sql_audit.view': { zhHans: '查看 SQL 审计', enUS: 'View SQL audit' },
  'database.operation_logs.view': {
    zhHans: '查看数据库操作日志',
    enUS: 'View database operation logs',
  },
  'file.upload_create': { zhHans: '上传或新建文件', enUS: 'Upload or create files' },
  'file.move_create': { zhHans: '新建或移动文件夹', enUS: 'Create or move folders' },
  'file.preview': { zhHans: '预览/下载文件', enUS: 'Preview/download files' },
  'file.upload': { zhHans: '上传文件', enUS: 'Upload files' },
  'file.text.create': { zhHans: '新建文本文件', enUS: 'Create text files' },
  'file.update': { zhHans: '编辑文件', enUS: 'Edit files' },
  'file.delete': { zhHans: '删除文件', enUS: 'Delete files' },
  'file.move': { zhHans: '移动文件', enUS: 'Move files' },
  'file.folder.manage': { zhHans: '管理文件夹', enUS: 'Manage folders' },
};

const getPermissionFallbackName = (code: string, locale?: string) => {
  const fallback = permissionFallbackNames[code];
  if (!fallback) return undefined;
  return isChineseLocale(locale) ? fallback.zhHans : fallback.enUS;
};

const readablePermissionSegments = (code: string, locale?: string) => {
  const labels = isChineseLocale(locale) ? zhSegmentLabels : enSegmentLabels;
  return code
    .split(/[._]/)
    .filter(Boolean)
    .map(part => labels[part] ?? part.replaceAll('_', ' '))
    .join(' / ');
};

export const formatPermissionFallbackLabel = (code: string, locale?: string) =>
  getPermissionFallbackName(code, locale) ?? readablePermissionSegments(code, locale);

export const formatPermissionFallbackDescription = (code: string, locale?: string) => {
  const label = formatPermissionFallbackLabel(code, locale);
  return isChineseLocale(locale) ? `允许${label}。` : `Allows ${label.toLowerCase()}.`;
};

/**
 * All permission codes as a const array for type derivation
 */
export const ALL_PERMISSION_CODES = [
  // Agent permissions
  'agent.view',
  'agent.manage',
  'agent.create',
  'agent.update',
  'agent.delete',
  'agent.move',
  'agent.publish',
  'agent.runtime_access.manage',
  'agent.logs.view',
  // Workflow permissions
  'workflow.view',
  'workflow.create',
  'workflow.update',
  'workflow.delete',
  'workflow.move',
  'workflow.import',
  'workflow.run.draft',
  'workflow.publish',
  'workflow.runtime_access.manage',
  'workflow.logs.view',
  // Knowledge base permissions
  'knowledge_base.view',
  'knowledge_base.manage',
  'knowledge_base.retrieval_test',
  'knowledge_base.folder_manage',
  'knowledge_base.create',
  'knowledge_base.update',
  'knowledge_base.delete',
  'knowledge_base.move',
  'knowledge_base.document.view',
  'knowledge_base.document.create',
  'knowledge_base.document.update',
  'knowledge_base.document.delete',
  'knowledge_base.segment.update',
  'knowledge_base.segment.delete',
  'knowledge_base.index.manage',
  'knowledge_base.graph.view',
  'knowledge_base.graph.manage',
  // Database permissions
  'database.view',
  'database.manage',
  'database.data_edit',
  'database.ai_query',
  'database.create',
  'database.update',
  'database.delete',
  'database.move',
  'database.schema.view',
  'database.schema.manage',
  'database.record.view',
  'database.record.create',
  'database.record.update',
  'database.record.delete',
  'database.import.analyze',
  'database.import.execute',
  'database.ai_query.read',
  'database.sql_audit.view',
  'database.operation_logs.view',
  // File permissions
  'file.view',
  'file.manage',
  'file.upload_create',
  'file.move_create',
  'file.preview',
  'file.upload',
  'file.text.create',
  'file.update',
  'file.delete',
  'file.move',
  'file.folder.manage',
] as const;

/**
 * Union type of all permission codes derived from the const array
 */
export type PermissionCode = (typeof ALL_PERMISSION_CODES)[number];

const ALL_PERMISSION_CODE_VALUES = new Set<string>(ALL_PERMISSION_CODES);

const isPermissionCode = (permission: string): permission is PermissionCode =>
  ALL_PERMISSION_CODE_VALUES.has(permission);

export const DEPRECATED_ASSET_PERMISSION_CODES = [
  'agent.manage',
  'knowledge_base.view',
  'knowledge_base.manage',
  'database.view',
  'database.manage',
  'file.view',
  'file.manage',
] as const satisfies readonly PermissionCode[];

export type DeprecatedAssetPermissionCode = (typeof DEPRECATED_ASSET_PERMISSION_CODES)[number];

const DEPRECATED_ASSET_PERMISSION_CODE_VALUES = new Set<string>(DEPRECATED_ASSET_PERMISSION_CODES);

export const COMPATIBILITY_PERMISSION_CODES = [
  'database.data_edit',
  'database.ai_query',
  'file.upload_create',
  'file.move_create',
] as const satisfies readonly PermissionCode[];

export type CompatibilityPermissionCode = (typeof COMPATIBILITY_PERMISSION_CODES)[number];

const COMPATIBILITY_PERMISSION_CODE_VALUES = new Set<string>(COMPATIBILITY_PERMISSION_CODES);

const COMPATIBILITY_PERMISSION_EXPANSIONS = {
  'database.data_edit': [
    'database.record.create',
    'database.record.update',
    'database.record.delete',
    'database.import.execute',
  ],
  'database.ai_query': ['database.ai_query.read'],
  'file.upload_create': ['file.upload', 'file.text.create'],
  'file.move_create': ['file.move', 'file.folder.manage'],
} as const satisfies Record<CompatibilityPermissionCode, readonly PermissionCode[]>;

export const GOVERNANCE_PERMISSION_CODES = [] as const satisfies readonly PermissionCode[];

const GOVERNANCE_PERMISSION_CODE_VALUES = new Set<string>(GOVERNANCE_PERMISSION_CODES);

const isRetiredWorkspacePermissionCode = (permission: string) =>
  permission.startsWith('prompt.') ||
  permission.startsWith('content_parse.') ||
  permission.startsWith('dashboard.') ||
  permission.startsWith('workspace.');

export const SELECTABLE_PERMISSION_CODES = ALL_PERMISSION_CODES.filter(
  code =>
    !isRetiredWorkspacePermissionCode(code) &&
    !DEPRECATED_ASSET_PERMISSION_CODE_VALUES.has(code) &&
    !COMPATIBILITY_PERMISSION_CODE_VALUES.has(code) &&
    !GOVERNANCE_PERMISSION_CODE_VALUES.has(code)
) as Array<Exclude<PermissionCode, DeprecatedAssetPermissionCode | CompatibilityPermissionCode>>;

const SELECTABLE_PERMISSION_CODE_VALUES = new Set<string>(SELECTABLE_PERMISSION_CODES);

export const PERMISSION_DEPENDENCIES: Partial<Record<PermissionCode, readonly PermissionCode[]>> = {
  'agent.create': ['agent.view'],
  'agent.update': ['agent.view'],
  'agent.delete': ['agent.view'],
  'agent.move': ['agent.view'],
  'agent.publish': ['agent.view'],
  'agent.runtime_access.manage': ['agent.view'],
  'agent.logs.view': ['agent.view'],
  'workflow.create': ['workflow.view'],
  'workflow.import': ['workflow.view'],
  'workflow.update': ['workflow.view'],
  'workflow.delete': ['workflow.view'],
  'workflow.move': ['workflow.view'],
  'workflow.run.draft': ['workflow.view'],
  'workflow.publish': ['workflow.view'],
  'workflow.runtime_access.manage': ['workflow.view'],
  'workflow.logs.view': ['workflow.view'],
  'database.schema.manage': ['database.schema.view'],
  'database.create': ['database.schema.view'],
  'database.record.view': ['database.schema.view'],
  'database.record.create': ['database.schema.view', 'database.record.view'],
  'database.record.update': ['database.schema.view', 'database.record.view'],
  'database.record.delete': ['database.schema.view', 'database.record.view'],
  'database.import.execute': ['database.import.analyze'],
  'knowledge_base.create': ['knowledge_base.document.view'],
  'knowledge_base.document.create': ['knowledge_base.document.view'],
  'knowledge_base.document.update': ['knowledge_base.document.view'],
  'knowledge_base.document.delete': ['knowledge_base.document.view'],
  'knowledge_base.segment.update': ['knowledge_base.document.view'],
  'knowledge_base.segment.delete': ['knowledge_base.document.view'],
  'knowledge_base.graph.manage': ['knowledge_base.graph.view'],
};

export const getMissingPermissionDependencies = (
  currentPermissions: Iterable<string>,
  requestedPermissions: Iterable<string>
): PermissionCode[] => {
  const current = new Set(currentPermissions);
  const missing: PermissionCode[] = [];
  const queue: PermissionCode[] = [];
  const queued = new Set<string>();

  for (const permission of requestedPermissions) {
    if (!isPermissionCode(permission) || queued.has(permission)) continue;
    queue.push(permission);
    queued.add(permission);
    current.add(permission);
  }

  for (let index = 0; index < queue.length; index += 1) {
    const dependencies = PERMISSION_DEPENDENCIES[queue[index]] ?? [];
    for (const dependency of dependencies) {
      if (!SELECTABLE_PERMISSION_CODE_VALUES.has(dependency) || current.has(dependency)) continue;

      current.add(dependency);
      missing.push(dependency);

      if (!queued.has(dependency)) {
        queue.push(dependency);
        queued.add(dependency);
      }
    }
  }

  return missing;
};

export const normalizeSelectablePermissionCodes = (
  permissions?: readonly string[] | null
): string[] => {
  if (!permissions?.length) return [];

  const normalized: string[] = [];
  const seen = new Set<string>();
  for (const permission of permissions) {
    if (COMPATIBILITY_PERMISSION_CODE_VALUES.has(permission)) {
      for (const mappedPermission of COMPATIBILITY_PERMISSION_EXPANSIONS[
        permission as CompatibilityPermissionCode
      ]) {
        if (seen.has(mappedPermission)) continue;
        seen.add(mappedPermission);
        normalized.push(mappedPermission);
      }
      continue;
    }

    if (
      isRetiredWorkspacePermissionCode(permission) ||
      DEPRECATED_ASSET_PERMISSION_CODE_VALUES.has(permission) ||
      GOVERNANCE_PERMISSION_CODE_VALUES.has(permission) ||
      seen.has(permission)
    ) {
      continue;
    }
    seen.add(permission);
    normalized.push(permission);
  }

  return normalized;
};

// Action-level permission matrix. Page groups are entry visibility only; concrete
// buttons and direct routes should use the matching action instead of manage groups.
export const AGENT_PERMISSION_ACTIONS = {
  page: [
    'agent.view',
    'agent.create',
    'agent.logs.view',
    'agent.update',
    'agent.delete',
    'agent.move',
    'agent.publish',
    'agent.runtime_access.manage',
  ],
  view: ['agent.view'],
  create: ['agent.create'],
  import: [],
  update: ['agent.update'],
  delete: ['agent.delete'],
  lock: [],
  move: ['agent.move'],
  copy: [],
  export: [],
  publish: ['agent.publish'],
  runtimeConfigManage: ['agent.update'],
  runtimeAccessManage: ['agent.runtime_access.manage'],
  logsView: ['agent.logs.view'],
  statsView: [],
  conversationView: [],
  conversationManage: [],
} as const satisfies Record<string, readonly PermissionCode[]>;

export const WORKFLOW_PERMISSION_ACTIONS = {
  page: [
    'workflow.create',
    'workflow.import',
    'workflow.view',
    'workflow.logs.view',
    'workflow.update',
    'workflow.delete',
    'workflow.move',
    'workflow.run.draft',
    'workflow.publish',
    'workflow.runtime_access.manage',
  ],
  create: ['workflow.create'],
  import: ['workflow.import'],
  view: ['workflow.view'],
  update: ['workflow.update'],
  delete: ['workflow.delete'],
  move: ['workflow.move'],
  copy: [],
  export: ['workflow.import'],
  runDraft: ['workflow.run.draft'],
  publish: ['workflow.publish'],
  runtimeConfigManage: ['workflow.update'],
  runtimeAccessManage: ['workflow.runtime_access.manage'],
  logsView: ['workflow.logs.view'],
  statsView: [],
} as const satisfies Record<string, readonly PermissionCode[]>;

export const KNOWLEDGE_BASE_PERMISSION_ACTIONS = {
  page: [
    'knowledge_base.create',
    'knowledge_base.folder_manage',
    'knowledge_base.retrieval_test',
    'knowledge_base.document.view',
    'knowledge_base.graph.view',
    'knowledge_base.update',
    'knowledge_base.delete',
    'knowledge_base.move',
    'knowledge_base.document.create',
    'knowledge_base.document.update',
    'knowledge_base.document.delete',
    'knowledge_base.segment.update',
    'knowledge_base.segment.delete',
    'knowledge_base.index.manage',
    'knowledge_base.graph.manage',
  ],
  create: ['knowledge_base.create'],
  update: ['knowledge_base.update'],
  delete: ['knowledge_base.delete'],
  move: ['knowledge_base.move'],
  folderView: ['knowledge_base.document.view'],
  folderManage: ['knowledge_base.folder_manage'],
  retrievalTest: ['knowledge_base.retrieval_test'],
  documentView: ['knowledge_base.document.view'],
  documentCreate: ['knowledge_base.document.create'],
  documentUpdate: ['knowledge_base.document.update'],
  documentDelete: ['knowledge_base.document.delete'],
  segmentView: ['knowledge_base.document.view'],
  segmentUpdate: ['knowledge_base.segment.update'],
  segmentDelete: ['knowledge_base.segment.delete'],
  indexManage: ['knowledge_base.index.manage'],
  graphView: ['knowledge_base.graph.view'],
  graphManage: ['knowledge_base.graph.manage'],
} as const satisfies Record<string, readonly PermissionCode[]>;

export const DATABASE_PERMISSION_ACTIONS = {
  page: [
    'database.create',
    'database.update',
    'database.delete',
    'database.move',
    'database.schema.view',
    'database.schema.manage',
    'database.record.view',
    'database.record.create',
    'database.record.update',
    'database.record.delete',
    'database.import.analyze',
    'database.import.execute',
    'database.operation_logs.view',
    'database.sql_audit.view',
    'database.ai_query.read',
  ],
  create: ['database.create'],
  update: ['database.update'],
  delete: ['database.delete'],
  move: ['database.move'],
  schemaView: ['database.schema.view'],
  schemaManage: ['database.schema.manage'],
  recordView: ['database.record.view'],
  recordCreate: ['database.record.create'],
  recordUpdate: ['database.record.update'],
  recordDelete: ['database.record.delete'],
  importAnalyze: ['database.import.analyze'],
  importExecute: ['database.import.execute'],
  importErrorsView: ['database.import.analyze', 'database.import.execute'],
  guardPolicyManage: ['database.schema.manage'],
  tablePromptView: ['database.schema.view'],
  tablePromptManage: ['database.schema.manage'],
  operationLogsView: ['database.operation_logs.view'],
  sqlAuditView: ['database.sql_audit.view'],
  aiQueryRead: ['database.ai_query.read'],
  aiQueryWrite: [],
} as const satisfies Record<string, readonly PermissionCode[]>;

export const FILE_PERMISSION_ACTIONS = {
  page: [],
  preview: ['file.preview'],
  folderView: [],
  relatedView: [],
  download: ['file.preview'],
  upload: ['file.upload'],
  textCreate: ['file.text.create'],
  update: ['file.update'],
  delete: ['file.delete'],
  move: ['file.move'],
  archive: [],
  folderManage: ['file.folder.manage'],
  shareManage: [],
  favoriteManage: [],
} as const satisfies Record<string, readonly PermissionCode[]>;

export const AGENT_ASSET_VISIBLE_PERMISSION_CODES = [
  ...AGENT_PERMISSION_ACTIONS.page,
  ...WORKFLOW_PERMISSION_ACTIONS.page,
] as const satisfies readonly PermissionCode[];

export const AGENT_VISIBLE_PERMISSION_CODES = AGENT_PERMISSION_ACTIONS.page;

export const AGENT_MANAGE_PERMISSION_CODES = [
  ...AGENT_PERMISSION_ACTIONS.update,
  ...AGENT_PERMISSION_ACTIONS.delete,
  ...AGENT_PERMISSION_ACTIONS.lock,
  ...AGENT_PERMISSION_ACTIONS.move,
  ...AGENT_PERMISSION_ACTIONS.copy,
  ...AGENT_PERMISSION_ACTIONS.export,
  ...AGENT_PERMISSION_ACTIONS.publish,
  ...AGENT_PERMISSION_ACTIONS.runtimeConfigManage,
  ...AGENT_PERMISSION_ACTIONS.runtimeAccessManage,
  ...AGENT_PERMISSION_ACTIONS.conversationManage,
] as const satisfies readonly PermissionCode[];

export const WORKFLOW_VISIBLE_PERMISSION_CODES = WORKFLOW_PERMISSION_ACTIONS.page;

export const WORKFLOW_MANAGE_PERMISSION_CODES = [
  ...WORKFLOW_PERMISSION_ACTIONS.create,
  ...WORKFLOW_PERMISSION_ACTIONS.update,
  ...WORKFLOW_PERMISSION_ACTIONS.delete,
  ...WORKFLOW_PERMISSION_ACTIONS.move,
  ...WORKFLOW_PERMISSION_ACTIONS.copy,
  ...WORKFLOW_PERMISSION_ACTIONS.import,
  ...WORKFLOW_PERMISSION_ACTIONS.export,
  ...WORKFLOW_PERMISSION_ACTIONS.runDraft,
  ...WORKFLOW_PERMISSION_ACTIONS.publish,
  ...WORKFLOW_PERMISSION_ACTIONS.runtimeConfigManage,
  ...WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage,
] as const satisfies readonly PermissionCode[];

export const KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES = KNOWLEDGE_BASE_PERMISSION_ACTIONS.page;

export const KNOWLEDGE_BASE_READ_PERMISSION_CODES = [
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentView,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphView,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.update,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.delete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.move,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.segmentUpdate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.segmentDelete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.folderManage,
] as const satisfies readonly PermissionCode[];

export const KNOWLEDGE_BASE_MANAGE_PERMISSION_CODES = [
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.create,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.update,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.delete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.move,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.folderManage,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.retrievalTest,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentCreate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentUpdate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.documentDelete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.segmentUpdate,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.segmentDelete,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.indexManage,
  ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage,
] as const satisfies readonly PermissionCode[];

export const DATABASE_VISIBLE_PERMISSION_CODES = DATABASE_PERMISSION_ACTIONS.page;

export const DATABASE_READ_BINDING_PERMISSION_CODES = [
  ...DATABASE_PERMISSION_ACTIONS.aiQueryRead,
  ...DATABASE_PERMISSION_ACTIONS.recordView,
] as const satisfies readonly PermissionCode[];

export const DATABASE_TABLE_METADATA_PERMISSION_CODES = [
  ...DATABASE_PERMISSION_ACTIONS.schemaView,
  ...DATABASE_PERMISSION_ACTIONS.schemaManage,
  ...DATABASE_PERMISSION_ACTIONS.recordView,
  ...DATABASE_PERMISSION_ACTIONS.recordCreate,
  ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
  ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ...DATABASE_PERMISSION_ACTIONS.importAnalyze,
  ...DATABASE_PERMISSION_ACTIONS.importExecute,
  ...DATABASE_PERMISSION_ACTIONS.aiQueryRead,
] as const satisfies readonly PermissionCode[];

export const DATABASE_MANAGE_PERMISSION_CODES = [
  ...DATABASE_PERMISSION_ACTIONS.create,
  ...DATABASE_PERMISSION_ACTIONS.update,
  ...DATABASE_PERMISSION_ACTIONS.delete,
  ...DATABASE_PERMISSION_ACTIONS.move,
  ...DATABASE_PERMISSION_ACTIONS.schemaManage,
  ...DATABASE_PERMISSION_ACTIONS.recordCreate,
  ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
  ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ...DATABASE_PERMISSION_ACTIONS.importAnalyze,
  ...DATABASE_PERMISSION_ACTIONS.importExecute,
  ...DATABASE_PERMISSION_ACTIONS.tablePromptManage,
] as const satisfies readonly PermissionCode[];

export const FILE_VISIBLE_PERMISSION_CODES = FILE_PERMISSION_ACTIONS.page;

export const FILE_MANAGE_PERMISSION_CODES = [
  ...FILE_PERMISSION_ACTIONS.upload,
  ...FILE_PERMISSION_ACTIONS.textCreate,
  ...FILE_PERMISSION_ACTIONS.update,
  ...FILE_PERMISSION_ACTIONS.delete,
  ...FILE_PERMISSION_ACTIONS.move,
  ...FILE_PERMISSION_ACTIONS.folderManage,
] as const satisfies readonly PermissionCode[];

const permissionItem = (code: PermissionCode): PermissionItem => ({
  code,
  name: `permissions.${code}.name`,
  description: `permissions.${code}.description`,
});

const permissionsWithPrefix = (prefix: string): PermissionItem[] =>
  SELECTABLE_PERMISSION_CODES.filter(code => code.startsWith(`${prefix}.`)).map(permissionItem);

export const PERMISSION_MODULES: PermissionModule[] = [
  {
    key: 'agent',
    title: 'permissions.modules.agent',
    permissions: permissionsWithPrefix('agent'),
  },
  {
    key: 'workflow',
    title: 'permissions.modules.workflow',
    permissions: permissionsWithPrefix('workflow'),
  },
  {
    key: 'knowledge',
    title: 'permissions.modules.knowledge',
    permissions: permissionsWithPrefix('knowledge_base'),
  },
  {
    key: 'database',
    title: 'permissions.modules.database',
    permissions: permissionsWithPrefix('database'),
  },
  {
    key: 'file',
    title: 'permissions.modules.file',
    permissions: permissionsWithPrefix('file'),
  },
];
