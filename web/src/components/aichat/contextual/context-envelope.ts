import type {
  AIChatRuntimeTransport,
  AIChatStreamCallbacks,
} from '@/components/chat/transports/aichat-transport';
import { aichatTransport } from '@/components/chat/transports/aichat-transport';
import type {
  AIChatAssetOperationAudit,
  AIChatChatRequest,
  AIChatClientActionRequiredEventData,
  AIChatClientActionResultRequest,
  AIChatRegenerateMessageRequest,
  AIChatSkillArtifactCreatedEventData,
  AIChatSkillCallEndEventData,
  AIChatToolGovernanceAssetRef,
  AIChatToolGovernanceDecisionRequest,
} from '@/services/types/aichat';
import type {
  AIChatCapabilityDescriptor,
  AIChatCapabilityRisk,
  AIChatContextItem,
  AIChatContextMetadata,
  AIChatOperationCapability,
  AIChatOperationContext,
  AIChatOperationMetadata,
  AIChatOperationMetadataValue,
  AIChatOperationRelation,
  AIChatOperationResource,
} from './types';

const MAX_CONTEXT_ITEMS = 8;
const MAX_METADATA_KEYS = 8;
const MAX_FIELD_LENGTH = 260;
const MAX_CAPABILITY_SUMMARY = 8;
const MAX_OPERATION_RESOURCES = 24;
const MAX_OPERATION_CAPABILITIES = 32;
const MAX_OPERATION_CAPABILITIES_PER_RESOURCE = 8;
const MAX_OPERATION_RELATIONS_PER_RESOURCE = 8;
const MAX_OPERATION_FIELD_LENGTH = 160;
const MAX_OPERATION_METADATA_KEYS = 32;
const MAX_OPERATION_METADATA_VALUE_LENGTH = 3200;
const MAX_OPERATION_ID_LENGTH = 120;

export interface ContextualAIChatTransportOptions {
  onAssetToolSuccess?: (payload: AIChatSkillCallEndEventData) => void;
  onAssetOperationSuccess?: (operation: ContextualAIChatAssetOperation) => void;
  onPageNavigationRequested?: (request: ContextualAIChatPageNavigationRequest) => void;
  onClientActionRequired?: (request: ContextualAIChatClientActionRequest) => void;
}

export type ContextualAIChatAssetOperationEffect =
  | 'create'
  | 'update'
  | 'delete'
  | 'publish'
  | 'invoke'
  | 'schedule'
  | 'external_send'
  | 'unknown';

export type ContextualAIChatAssetOperationSource = 'skill_call' | 'skill_artifact';

export interface ContextualAIChatAssetOperation {
  assetType: string;
  effect: ContextualAIChatAssetOperationEffect;
  source: ContextualAIChatAssetOperationSource;
  skillId: string;
  toolName: string;
  toolId?: string;
  assetId?: string;
  assetName?: string;
  payload: AIChatSkillCallEndEventData | AIChatSkillArtifactCreatedEventData;
}

export interface ContextualAIChatPageNavigationRequest {
  href: string;
  label?: string;
  reason?: string;
  payload: AIChatSkillCallEndEventData;
}

export interface ContextualAIChatClientActionRequest {
  actionId: string;
  actionType: string;
  conversationId: string;
  messageId: string;
  href?: string;
  label?: string;
  reason?: string;
  payload: AIChatClientActionRequiredEventData;
}

export const ZGI_CONSOLE_SITE_MAP = [
  { href: '/console', label: 'Home', purpose: 'workspace overview and entry point' },
  { href: '/console/work/chat', label: 'Conversations', purpose: 'chat workbench' },
  { href: '/console/work/image', label: 'Images', purpose: 'image/drawing workbench' },
  { href: '/console/work/app', label: 'Apps', purpose: 'web app workbench' },
  { href: '/console/work/task', label: 'Scheduled Tasks', purpose: 'scheduled task management' },
  {
    href: '/console/agents',
    label: 'Agents',
    purpose: 'create, configure, debug, and publish agents',
  },
  {
    href: '/console/dataset',
    label: 'Knowledge Bases',
    purpose: 'knowledge base and document assets',
  },
  { href: '/console/db', label: 'Databases', purpose: 'database sources and table records' },
  { href: '/console/files', label: 'Files', purpose: 'uploaded files and managed workspace files' },
  { href: '/console/prompts', label: 'Prompts', purpose: 'prompt library' },
  {
    href: '/console/developer/content-parse',
    label: 'File Recognition',
    purpose: 'content parsing and recognition tools',
  },
  { href: '/console/workspace', label: 'Workspace', purpose: 'workspace administration' },
  {
    href: '/console/workspace/members',
    label: 'Workspace Members',
    purpose: 'workspace member management',
  },
  {
    href: '/console/workspace/settings',
    label: 'Workspace Settings',
    purpose: 'workspace configuration',
  },
  { href: '/console/settings', label: 'System Settings', purpose: 'account and system settings' },
] as const;

const ZGI_CONSOLE_EXACT_ROUTES: ReadonlySet<string> = new Set(
  ZGI_CONSOLE_SITE_MAP.map(route => route.href)
);
const ZGI_CONSOLE_DYNAMIC_ROUTES = [
  /^\/console\/agents\/[A-Za-z0-9_-]+\/(agent|workflow|logs|api|batch-test)$/,
  /^\/console\/dataset\/[A-Za-z0-9_-]+(\/(documents|graph|hit-testing|batch-testing|settings))?$/,
  /^\/console\/db\/[A-Za-z0-9_-]+(\/(record|search|table|import-excel))?$/,
  /^\/console\/db\/[A-Za-z0-9_-]+\/table\/[A-Za-z0-9_-]+$/,
  /^\/console\/prompts\/[A-Za-z0-9_-]+$/,
  /^\/console\/work\/app\/[A-Za-z0-9_-]+$/,
];

const ZGI_SYSTEM_CONTEXT_ITEM_ID = 'zgi.system_assistant';
const ZGI_SITE_MAP_SUMMARY = ZGI_CONSOLE_SITE_MAP.map(
  route => `${route.label}=${route.href}(${route.purpose})`
).join('; ');

const ZGI_SYSTEM_CONTEXT_ITEM: AIChatContextItem = {
  id: ZGI_SYSTEM_CONTEXT_ITEM_ID,
  type: 'custom',
  title: 'ZGI AIChat system assistant',
  description:
    'AIChat is the ZGI sidebar operation assistant. It can explain the current page, answer from registered page context, navigate to whitelisted internal console routes, and use enabled low-risk skills. High-risk asset mutations require supported governed tools and user approval and must not be promised when unavailable.',
  source: 'system',
  status: 'available',
  risk: 'low',
  capabilities: [
    {
      id: 'page.navigate',
      title: 'Navigate inside ZGI',
      description:
        'Switch the visible console page to a whitelisted internal route through console-navigator / navigate.',
      risk: 'low',
      status: 'available',
      permissions: ['console:navigate'],
    },
    {
      id: 'assistant.self_describe',
      title: 'Explain AIChat role and limits',
      description:
        'Describe AIChat as a ZGI operation assistant, including current page help, site-wide navigation, enabled skills, and high-risk operation limits.',
      risk: 'low',
      status: 'available',
    },
  ],
  metadata: {
    site_map: ZGI_SITE_MAP_SUMMARY,
    routing_tool: 'console-navigator / navigate',
    memory_boundary: 'AIChat account memory and Agent memory are separate',
    high_risk_limit:
      'Do not claim asset creation, deletion, publishing, workflow execution, scheduling, or agent mutation unless a supported governed tool result proves it.',
  },
};

const RISK_RANK: Record<AIChatCapabilityRisk, number> = {
  low: 1,
  medium: 2,
  high: 3,
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function compactText(value: string | undefined, limit = MAX_FIELD_LENGTH): string {
  const text = (value ?? '').replace(/\s+/g, ' ').trim();
  if (text.length <= limit) return text;
  return `${text.slice(0, limit).trim()}...`;
}

function compactOptionalText(
  value: string | undefined,
  limit = MAX_OPERATION_FIELD_LENGTH
): string | undefined {
  const text = compactText(value, limit);
  return text || undefined;
}

function formatMetadata(item: AIChatContextItem): string {
  const entries = Object.entries(item.metadata ?? {})
    .filter(([, value]) => value !== undefined && value !== null && `${value}`.trim() !== '')
    .slice(0, MAX_METADATA_KEYS);
  if (entries.length === 0) return '';
  return entries.map(([key, value]) => `${key}=${compactText(`${value}`, 120)}`).join(', ');
}

function uniqueValues(values: string[] | undefined, limit = MAX_OPERATION_RELATIONS_PER_RESOURCE) {
  const unique = Array.from(
    new Set((values ?? []).map(value => compactText(value, 80)).filter(Boolean))
  ).slice(0, limit);
  return unique.length > 0 ? unique : undefined;
}

function getHighestRisk(
  risks: Array<AIChatCapabilityRisk | undefined>
): AIChatCapabilityRisk | undefined {
  return risks
    .filter((risk): risk is AIChatCapabilityRisk => Boolean(risk))
    .sort((left, right) => RISK_RANK[right] - RISK_RANK[left])[0];
}

function formatCapabilities(item: AIChatContextItem): string {
  const capabilities = (item.capabilities ?? [])
    .filter(capability => capability.id.trim())
    .slice(0, MAX_CAPABILITY_SUMMARY);
  if (capabilities.length === 0) return '';

  const highestRisk = getHighestRisk(capabilities.map(capability => capability.risk));
  const summary = capabilities
    .map(capability => {
      const confirmation = capability.requiresConfirmation ? ', confirmation required' : '';
      return `${compactText(capability.id, 80)}(${capability.risk}${confirmation})`;
    })
    .join(', ');
  const omitted =
    (item.capabilities?.length ?? 0) > capabilities.length
      ? `; omitted=${(item.capabilities?.length ?? 0) - capabilities.length}`
      : '';

  return `capabilities=${summary}; highest_capability_risk=${highestRisk ?? 'low'}${omitted}`;
}

function buildResourceTypeInventory(items: AIChatContextItem[]): string {
  const counts = new Map<AIChatContextItem['type'], number>();
  items.forEach(item => {
    counts.set(item.type, (counts.get(item.type) ?? 0) + 1);
  });

  return Array.from(counts.entries())
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([type, count]) => `${type}=${count}`)
    .join(', ');
}

function metadataText(item: AIChatContextItem, key: string): string | undefined {
  const value = item.metadata?.[key];
  if (value === undefined || value === null || `${value}`.trim() === '') return undefined;
  return compactText(`${value}`, 80);
}

function isFileAsset(item: AIChatContextItem): boolean {
  return item.type === 'file' || item.metadata?.resource_kind === 'file';
}

function buildVisibleFileAssetSummary(items: AIChatContextItem[]): string {
  const fileItems = items.filter(isFileAsset);
  if (fileItems.length === 0) return '';

  const entries = fileItems
    .map((item, index) => {
      const order = metadataText(item, 'visible_index') ?? `${index + 1}`;
      const fileType =
        metadataText(item, 'file_type') ??
        metadataText(item, 'file_type_normalized') ??
        metadataText(item, 'extension');
      const fileTypeRank = metadataText(item, 'file_type_rank');
      const extensionRank = metadataText(item, 'extension_rank');
      const fileTypeSuffix = fileType ? ` (${fileType})` : '';
      const rankSummary = [
        fileTypeRank ? `file_type_rank=${fileTypeRank}` : '',
        extensionRank ? `extension_rank=${extensionRank}` : '',
      ]
        .filter(Boolean)
        .join(', ');
      const rankSuffix = rankSummary ? ` [${rankSummary}]` : '';
      return `${order}. ${compactText(item.title, 100)}${fileTypeSuffix}${rankSuffix}`;
    })
    .join(' | ');

  return compactText(
    `Visible file assets: count=${fileItems.length}; count and list only type=file/resource_kind=file as files; page, selection, log, and custom context items are not files. For typed ordinal requests such as second Excel, use file_type_rank/extension_rank among visible files of that type, not visible_index alone. ${entries}`,
    1400
  );
}

function sanitizeMetadataValue(
  value: AIChatContextMetadata[string]
): AIChatOperationMetadataValue | undefined {
  if (value === undefined) return undefined;
  if (value === null) return null;
  if (typeof value === 'string') {
    return compactOptionalText(value, MAX_OPERATION_METADATA_VALUE_LENGTH);
  }
  return value;
}

function sanitizeMetadata(
  metadata: AIChatContextMetadata | undefined
): AIChatOperationMetadata | undefined {
  const entries = Object.entries(metadata ?? {})
    .map(([key, value]) => {
      const sanitizedKey = compactOptionalText(key, 80);
      const sanitizedValue = sanitizeMetadataValue(value);
      if (!sanitizedKey || sanitizedValue === undefined) return null;
      return [sanitizedKey, sanitizedValue] as const;
    })
    .filter((entry): entry is readonly [string, AIChatOperationMetadataValue] => Boolean(entry))
    .slice(0, MAX_OPERATION_METADATA_KEYS);

  if (entries.length === 0) return undefined;
  return Object.fromEntries(entries);
}

function sanitizeRelation(
  relation: NonNullable<AIChatContextItem['relations']>[number]
): AIChatOperationRelation {
  return {
    relation_type: compactText(relation.type, MAX_OPERATION_ID_LENGTH),
    resource_type: relation.resourceType,
    resource_id: compactText(relation.resourceId, MAX_OPERATION_ID_LENGTH),
    title: compactOptionalText(relation.title),
    metadata: sanitizeMetadata(relation.metadata),
  };
}

function sanitizeCapability(
  item: AIChatContextItem,
  capability: AIChatCapabilityDescriptor
): AIChatOperationCapability {
  return {
    id: compactText(capability.id, MAX_OPERATION_ID_LENGTH),
    title: compactOptionalText(capability.title),
    description: compactOptionalText(capability.description),
    resource_id: compactText(item.id, MAX_OPERATION_ID_LENGTH),
    resource_type: item.type,
    risk: capability.risk,
    requires_confirmation: capability.requiresConfirmation || undefined,
    status: capability.status,
    permissions: uniqueValues(capability.permissions),
    metadata: sanitizeMetadata(capability.metadata),
  };
}

export function buildAIChatContextEnvelope(items: AIChatContextItem[]): string {
  const visibleItems = items.slice(0, MAX_CONTEXT_ITEMS);
  if (visibleItems.length === 0) return '';

  const lines = [
    'AIChat role: ZGI sidebar system assistant for helping users operate the ZGI console, not a generic chat-only bot.',
    `ZGI site map: ${ZGI_SITE_MAP_SUMMARY}.`,
    'Navigation ability: for user-requested page switches, use console-navigator / navigate with a whitelisted internal /console route; do not navigate to external URLs.',
    'Execution boundary: do not claim high-risk asset operations such as delete, publish, workflow run, scheduling, or agent mutation unless a supported governed tool result proves it in this turn.',
    'Current ZGI page context. Use it only to interpret this turn; do not save it as memory unless the user explicitly asks.',
    'Important: AIChat account memory and Agent memory are separate. Do not claim they are shared.',
    'Important: Workflow resources and Agent resources are distinct; a workflow binding does not make the workflow the Agent.',
    'Important: each numbered item has an explicit type. Count or list assets only when the item type or resource_kind matches the asset type the user asked for; page items are navigation/context, not user assets.',
    `Context item type inventory: ${buildResourceTypeInventory(items)}.`,
  ];
  const fileAssetSummary = buildVisibleFileAssetSummary(items);
  if (fileAssetSummary) {
    lines.push(fileAssetSummary);
  }

  visibleItems.forEach((item, index) => {
    const details = [
      `type=${item.type}`,
      `id=${item.id}`,
      `title=${compactText(item.title)}`,
      item.subtitle ? `subtitle=${compactText(item.subtitle)}` : '',
      item.description ? `description=${compactText(item.description)}` : '',
      item.href ? `href=${item.href}` : '',
      item.permissions?.length ? `permissions=${item.permissions.join(',')}` : '',
      item.status ? `status=${item.status}` : '',
      item.risk ? `risk=${item.risk}` : '',
      item.relations?.length ? `relations=${item.relations.length}` : '',
      formatCapabilities(item),
      formatMetadata(item),
    ].filter(Boolean);
    lines.push(`${index + 1}. ${details.join('; ')}`);
  });

  if (items.length > visibleItems.length) {
    lines.push(`Additional context items omitted: ${items.length - visibleItems.length}.`);
  }

  return lines.join('\n');
}

export function buildAIChatOperationContext(items: AIChatContextItem[]): AIChatOperationContext {
  const visibleItems = items.slice(0, MAX_OPERATION_RESOURCES);
  const capabilities: AIChatOperationCapability[] = [];
  let omittedCapabilityCount = 0;

  const resources: AIChatOperationResource[] = visibleItems.map(item => {
    const validCapabilities = (item.capabilities ?? []).filter(capability => capability.id.trim());
    const visibleCapabilities = validCapabilities.slice(0, MAX_OPERATION_CAPABILITIES_PER_RESOURCE);
    const capabilityIds: string[] = [];

    visibleCapabilities.forEach(capability => {
      if (capabilities.length >= MAX_OPERATION_CAPABILITIES) {
        omittedCapabilityCount += 1;
        return;
      }

      const sanitizedCapability = sanitizeCapability(item, capability);
      if (!sanitizedCapability.id) return;
      capabilities.push(sanitizedCapability);
      capabilityIds.push(sanitizedCapability.id);
    });
    omittedCapabilityCount += Math.max(0, validCapabilities.length - visibleCapabilities.length);

    const relations = (item.relations ?? [])
      .filter(relation => relation.type.trim() && relation.resourceId.trim())
      .slice(0, MAX_OPERATION_RELATIONS_PER_RESOURCE)
      .map(sanitizeRelation);

    return {
      resource_id: compactText(item.id, MAX_OPERATION_ID_LENGTH),
      resource_type: item.type,
      title: compactText(item.title, MAX_OPERATION_FIELD_LENGTH),
      subtitle: compactOptionalText(item.subtitle),
      href: compactOptionalText(item.href, MAX_OPERATION_FIELD_LENGTH),
      source: compactOptionalText(item.source, 80),
      status: item.status,
      risk: item.risk,
      permissions: uniqueValues(item.permissions),
      metadata: sanitizeMetadata(item.metadata),
      relations: relations.length > 0 ? relations : undefined,
      capability_ids: capabilityIds.length > 0 ? capabilityIds : undefined,
    };
  });

  const omittedResourceCount = Math.max(0, items.length - visibleItems.length);
  if (omittedResourceCount > 0) {
    omittedCapabilityCount += items
      .slice(MAX_OPERATION_RESOURCES)
      .reduce(
        (count, item) =>
          count + (item.capabilities ?? []).filter(capability => capability.id.trim()).length,
        0
      );
  }
  const highestRisk = getHighestRisk([
    ...resources.map(resource => resource.risk),
    ...capabilities.map(capability => capability.risk),
  ]);

  return {
    schema: 'zgi.aichat.operation_context.v1',
    version: 1,
    resources,
    capabilities,
    risk_summary: {
      level: highestRisk,
      requires_confirmation: capabilities.some(capability => capability.requires_confirmation),
    },
    summary: {
      resource_count: resources.length,
      capability_count: capabilities.length,
      highest_risk: highestRisk,
      omitted_resource_count: omittedResourceCount,
      omitted_capability_count: omittedCapabilityCount,
    },
  };
}

function mergeAIChatOperationContext(
  pageContext: AIChatOperationContext,
  requestContext: unknown
): AIChatOperationContext | undefined {
  const hasPageContext = pageContext.resources.length > 0 || pageContext.capabilities.length > 0;
  if (!isRecord(requestContext)) {
    return hasPageContext ? pageContext : undefined;
  }

  const incoming = requestContext as Partial<AIChatOperationContext>;
  return {
    ...incoming,
    schema: 'zgi.aichat.operation_context.v1',
    version: 1,
    resources: hasPageContext
      ? pageContext.resources
      : Array.isArray(incoming.resources)
        ? incoming.resources
        : [],
    capabilities: hasPageContext
      ? pageContext.capabilities
      : Array.isArray(incoming.capabilities)
        ? incoming.capabilities
        : [],
    risk_summary: hasPageContext
      ? pageContext.risk_summary
      : (incoming.risk_summary ?? pageContext.risk_summary),
    summary: hasPageContext ? pageContext.summary : (incoming.summary ?? pageContext.summary),
    tool_governance: incoming.tool_governance ?? pageContext.tool_governance,
  };
}

function isSuccessfulSkillCall(payload: AIChatSkillCallEndEventData): boolean {
  return !payload.status || payload.status === 'success';
}

function textValue(value: unknown): string | undefined {
  if (typeof value !== 'string' && typeof value !== 'number') return undefined;
  const text = `${value}`.trim();
  return text || undefined;
}

function firstText(...values: unknown[]): string | undefined {
  for (const value of values) {
    const text = textValue(value);
    if (text) return text;
  }
  return undefined;
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return isRecord(value) ? value : undefined;
}

export function normalizeZGIConsoleNavigationHref(rawHref: string | undefined): string | null {
  const text = rawHref?.trim();
  if (!text || text.includes('..') || /^https?:\/\//i.test(text) || text.startsWith('//')) {
    return null;
  }

  const [rawPath] = text.split(/[?#]/, 1);
  const path = `/${rawPath.replace(/^\/+/, '')}`.replace(/\/+$/, '') || '/';
  const normalizedPath = path === '' ? '/' : path;

  if (ZGI_CONSOLE_EXACT_ROUTES.has(normalizedPath)) {
    return normalizedPath;
  }
  if (ZGI_CONSOLE_DYNAMIC_ROUTES.some(pattern => pattern.test(normalizedPath))) {
    return normalizedPath;
  }
  return null;
}

function withZGISystemContextItems(items: AIChatContextItem[]): AIChatContextItem[] {
  return [ZGI_SYSTEM_CONTEXT_ITEM, ...items.filter(item => item.id !== ZGI_SYSTEM_CONTEXT_ITEM_ID)];
}

function normalizeToken(value: unknown): string | undefined {
  const text = textValue(value)
    ?.toLowerCase()
    .replace(/[^a-z0-9]+/g, '_');
  return text?.replace(/^_+|_+$/g, '') || undefined;
}

function normalizeAssetType(value: unknown): string | undefined {
  const token = normalizeToken(value);
  if (!token) return undefined;

  if (
    ['file', 'files', 'upload_file', 'managed_file', 'workspace_file', 'local_file'].includes(token)
  ) {
    return 'file';
  }
  if (['agent', 'agents', 'app', 'webapp', 'web_app'].includes(token)) return 'agent';
  if (['workflow', 'workflows', 'workflow_run', 'workflow_runs'].includes(token)) {
    return token.includes('run') ? 'workflow_run' : 'workflow';
  }
  if (['automation', 'automations', 'scheduled_task', 'task', 'tasks'].includes(token)) {
    return 'automation';
  }
  if (['knowledge', 'knowledge_base', 'dataset', 'datasets', 'document'].includes(token)) {
    return 'knowledge';
  }
  if (['database', 'databases', 'db', 'dbs', 'table', 'database_table'].includes(token)) {
    return 'database';
  }
  if (['prompt', 'prompts'].includes(token)) return 'prompt';
  if (['workspace', 'workspaces'].includes(token)) return 'workspace';

  return token;
}

function contextHandlesAssetType(items: AIChatContextItem[], assetType: string): boolean {
  const normalizedAssetType = normalizeAssetType(assetType);
  if (!normalizedAssetType) return false;

  return items.some(item =>
    item.hints?.handledAssetTypes?.some(
      handledAssetType => normalizeAssetType(handledAssetType) === normalizedAssetType
    )
  );
}

function inferEffectFromText(value: unknown): ContextualAIChatAssetOperationEffect {
  const token = normalizeToken(value);
  if (!token) return 'unknown';
  if (token.includes('delete') || token.includes('remove')) return 'delete';
  if (token.includes('publish')) return 'publish';
  if (token.includes('schedule')) return 'schedule';
  if (
    token.includes('invoke') ||
    token.includes('execute') ||
    token.includes('run') ||
    token.includes('call')
  ) {
    return 'invoke';
  }
  if (
    token.includes('update') ||
    token.includes('edit') ||
    token.includes('rename') ||
    token.includes('move')
  ) {
    return 'update';
  }
  if (
    token.includes('create') ||
    token.includes('add') ||
    token.includes('upload') ||
    token.includes('generate') ||
    token.includes('save')
  ) {
    return 'create';
  }
  return 'unknown';
}

function normalizeEffect(value: unknown): ContextualAIChatAssetOperationEffect {
  const token = normalizeToken(value);
  if (!token || token === 'none' || token === 'read' || token === 'list' || token === 'search') {
    return 'unknown';
  }
  if (
    token === 'create' ||
    token === 'update' ||
    token === 'delete' ||
    token === 'publish' ||
    token === 'invoke' ||
    token === 'schedule' ||
    token === 'external_send'
  ) {
    return token;
  }
  return inferEffectFromText(token);
}

function inferAssetTypeFromToolID(toolID: string | undefined): string | undefined {
  const token = normalizeToken(toolID);
  if (!token) return undefined;
  if (token.startsWith('file_')) return 'file';
  if (token.startsWith('agent_')) return 'agent';
  if (token.startsWith('workflow_')) return 'workflow';
  if (token.startsWith('automation_') || token.startsWith('task_')) return 'automation';
  if (token.startsWith('knowledge_') || token.startsWith('dataset_')) return 'knowledge';
  if (token.startsWith('database_') || token.startsWith('db_') || token.startsWith('table_')) {
    return 'database';
  }
  if (token.startsWith('prompt_')) return 'prompt';
  if (token.startsWith('workspace_')) return 'workspace';
  return undefined;
}

function firstAssetReferenceValue(
  assets: AIChatToolGovernanceAssetRef[],
  keys: Array<keyof AIChatToolGovernanceAssetRef>
): string | undefined {
  for (const asset of assets) {
    for (const key of keys) {
      const value = textValue(asset[key]);
      if (value) return value;
    }
  }
  return undefined;
}

function auditFromRecord(record: Record<string, unknown> | undefined) {
  return record as AIChatAssetOperationAudit | undefined;
}

function skillCallAuditRecords(payload: AIChatSkillCallEndEventData): AIChatAssetOperationAudit[] {
  const feedback = recordValue(payload.governance?.model_feedback);
  const feedbackAudit = auditFromRecord(recordValue(feedback?.asset_operation_audit));
  return [
    payload.asset_operation_audit,
    payload.governance?.asset_operation_audit,
    feedbackAudit,
  ].filter((audit): audit is AIChatAssetOperationAudit => Boolean(audit));
}

function artifactAuditRecords(
  payload: AIChatSkillArtifactCreatedEventData
): AIChatAssetOperationAudit[] {
  return [payload.asset_operation_audit, payload.file?.asset_operation_audit].filter(
    (audit): audit is AIChatAssetOperationAudit => Boolean(audit)
  );
}

function skillCallAssetReferences(
  payload: AIChatSkillCallEndEventData
): AIChatToolGovernanceAssetRef[] {
  return [
    ...(payload.governance?.assets ?? []),
    ...(payload.governance?.approval_event?.assets ?? []),
    ...skillCallAuditRecords(payload).flatMap(audit => audit.assets ?? []),
  ];
}

function artifactAssetReferences(
  payload: AIChatSkillArtifactCreatedEventData
): AIChatToolGovernanceAssetRef[] {
  return artifactAuditRecords(payload).flatMap(audit => audit.assets ?? []);
}

function hasManagedFileSignal(record: Record<string, unknown> | undefined): boolean {
  if (!record) return false;
  const target = normalizeToken(record.target);
  const transferMethod = normalizeToken(record.transfer_method);
  const downloadURL = textValue(record.download_url);
  const url = textValue(record.url);
  return (
    target === 'managed_file' ||
    target === 'file_management' ||
    target === 'workspace_file' ||
    transferMethod === 'local_file' ||
    Boolean(textValue(record.upload_file_id)) ||
    downloadURL?.includes('/console/api/files/') === true ||
    url?.includes('/console/api/files/') === true
  );
}

function skillCallHasManagedFileResult(payload: AIChatSkillCallEndEventData): boolean {
  const result = recordValue(payload.result);
  return hasManagedFileSignal(result) || hasManagedFileSignal(recordValue(result?.file));
}

function artifactHasManagedFileResult(payload: AIChatSkillArtifactCreatedEventData): boolean {
  const artifact = recordValue(payload);
  return hasManagedFileSignal(artifact) || hasManagedFileSignal(recordValue(payload.file));
}

function shouldEmitFileOperation(
  operation: Pick<ContextualAIChatAssetOperation, 'effect' | 'source'>,
  items: AIChatContextItem[],
  hasManagedFileResult: boolean
): boolean {
  if (operation.effect === 'delete') return true;
  if (operation.effect === 'create') return hasManagedFileResult;
  if (operation.effect === 'update') {
    return hasManagedFileResult || contextHandlesAssetType(items, 'file');
  }
  return true;
}

function isMutatingEffect(effect: ContextualAIChatAssetOperationEffect): boolean {
  return effect !== 'unknown';
}

function operationFromSkillCall(
  payload: AIChatSkillCallEndEventData,
  items: AIChatContextItem[]
): ContextualAIChatAssetOperation | null {
  if (!isSuccessfulSkillCall(payload)) return null;

  const audits = skillCallAuditRecords(payload);
  const primaryAudit = audits[0];
  const assets = skillCallAssetReferences(payload);
  const toolID = firstText(
    primaryAudit?.tool_id,
    payload.governance?.manifest?.tool_id,
    payload.governance?.approval_event?.tool_id
  );
  const result = recordValue(payload.result);
  const assetType =
    normalizeAssetType(
      firstText(
        primaryAudit?.asset_type,
        payload.governance?.manifest?.asset_type,
        payload.governance?.approval_event?.asset_type,
        firstAssetReferenceValue(assets, ['type'])
      )
    ) ??
    inferAssetTypeFromToolID(toolID) ??
    (payload.skill_id === 'file-generator' ? 'file' : undefined);
  const effect = normalizeEffect(
    firstText(
      primaryAudit?.effect,
      primaryAudit?.action,
      payload.governance?.manifest?.effect,
      payload.governance?.approval_event?.effect,
      toolID,
      payload.tool_name
    )
  );

  if (!assetType || !isMutatingEffect(effect)) return null;

  const hasManagedFileResult = skillCallHasManagedFileResult(payload);
  if (
    assetType === 'file' &&
    !shouldEmitFileOperation({ effect, source: 'skill_call' }, items, hasManagedFileResult)
  ) {
    return null;
  }

  return {
    assetType,
    effect,
    source: 'skill_call',
    skillId: payload.skill_id,
    toolName: payload.tool_name,
    toolId: toolID,
    assetId: firstText(
      result?.upload_file_id,
      result?.file_id,
      result?.id,
      firstAssetReferenceValue(assets, ['id'])
    ),
    assetName: firstText(
      result?.filename,
      result?.name,
      result?.title,
      firstAssetReferenceValue(assets, ['filename', 'file_name', 'name', 'title', 'label'])
    ),
    payload,
  };
}

function operationFromSkillArtifact(
  payload: AIChatSkillArtifactCreatedEventData,
  items: AIChatContextItem[]
): ContextualAIChatAssetOperation | null {
  const audits = artifactAuditRecords(payload);
  const primaryAudit = audits[0];
  const assets = artifactAssetReferences(payload);
  const toolID = firstText(primaryAudit?.tool_id);
  const artifact = recordValue(payload);
  const file = recordValue(payload.file);
  const assetType =
    normalizeAssetType(
      firstText(primaryAudit?.asset_type, firstAssetReferenceValue(assets, ['type']))
    ) ??
    inferAssetTypeFromToolID(toolID) ??
    'file';
  const effect = normalizeEffect(
    firstText(primaryAudit?.effect, primaryAudit?.action, toolID, payload.tool_name)
  );

  if (!assetType || !isMutatingEffect(effect)) return null;

  const hasManagedFileResult = artifactHasManagedFileResult(payload);
  if (
    assetType === 'file' &&
    !shouldEmitFileOperation({ effect, source: 'skill_artifact' }, items, hasManagedFileResult)
  ) {
    return null;
  }

  return {
    assetType,
    effect,
    source: 'skill_artifact',
    skillId: payload.skill_id,
    toolName: payload.tool_name,
    toolId: toolID,
    assetId: firstText(
      artifact?.upload_file_id,
      file?.upload_file_id,
      artifact?.file_id,
      file?.file_id,
      artifact?.id,
      file?.id,
      firstAssetReferenceValue(assets, ['id'])
    ),
    assetName: firstText(
      artifact?.filename,
      file?.filename,
      artifact?.name,
      file?.name,
      firstAssetReferenceValue(assets, ['filename', 'file_name', 'name', 'title', 'label'])
    ),
    payload,
  };
}

function pageNavigationRequestFromSkillCall(
  payload: AIChatSkillCallEndEventData
): ContextualAIChatPageNavigationRequest | null {
  if (!isSuccessfulSkillCall(payload)) return null;
  if (payload.skill_id !== 'console-navigator' || payload.tool_name !== 'navigate') return null;

  const result = recordValue(payload.result);
  if (textValue(result?.event_type) !== 'page_navigation_requested') return null;

  const href = normalizeZGIConsoleNavigationHref(textValue(result?.href));
  if (!href) return null;

  return {
    href,
    label: textValue(result?.label),
    reason: textValue(result?.reason),
    payload,
  };
}

function clientActionRequestFromPayload(
  payload: AIChatClientActionRequiredEventData
): ContextualAIChatClientActionRequest | null {
  const actionId = textValue(payload.action_id);
  const actionType = textValue(payload.action_type);
  const conversationId = textValue(payload.conversation_id);
  const messageId = textValue(payload.message_id);
  if (!actionId || !actionType || !conversationId || !messageId) return null;

  const href =
    actionType === 'route_navigation'
      ? normalizeZGIConsoleNavigationHref(textValue(payload.href))
      : textValue(payload.href);

  return {
    actionId,
    actionType,
    conversationId,
    messageId,
    href: href || undefined,
    label: textValue(payload.label),
    reason: textValue(payload.reason),
    payload,
  };
}

function wrapContextualCallbacks(
  callbacks: AIChatStreamCallbacks,
  getContextItems: () => AIChatContextItem[],
  options?: ContextualAIChatTransportOptions
): AIChatStreamCallbacks {
  if (
    !options?.onAssetToolSuccess &&
    !options?.onAssetOperationSuccess &&
    !options?.onPageNavigationRequested &&
    !options?.onClientActionRequired
  ) {
    return callbacks;
  }

  return {
    ...callbacks,
    onSkillCallEnd: (payload, eventId) => {
      callbacks.onSkillCallEnd(payload, eventId);
      const navigationRequest = pageNavigationRequestFromSkillCall(payload);
      if (navigationRequest) {
        options?.onPageNavigationRequested?.(navigationRequest);
      }
      const operation = operationFromSkillCall(payload, getContextItems());
      if (!operation) return;
      options?.onAssetOperationSuccess?.(operation);
      if (operation.assetType === 'file') {
        options.onAssetToolSuccess?.(payload);
      }
    },
    onClientActionRequired: (payload, eventId) => {
      callbacks.onClientActionRequired?.(payload, eventId);
      const request = clientActionRequestFromPayload(payload);
      if (request) {
        options?.onClientActionRequired?.(request);
      }
    },
    onSkillArtifactCreated: (payload, eventId) => {
      callbacks.onSkillArtifactCreated(payload, eventId);
      const operation = operationFromSkillArtifact(payload, getContextItems());
      if (operation) {
        options?.onAssetOperationSuccess?.(operation);
      }
    },
  };
}

export function createContextualAIChatTransport(
  getContextItems: () => AIChatContextItem[],
  options?: ContextualAIChatTransportOptions
): AIChatRuntimeTransport {
  const base = aichatTransport;
  return {
    listConversations: base.listConversations.bind(base),
    getConversation: base.getConversation.bind(base),
    listMessages: base.listMessages.bind(base),
    refreshConversation: base.refreshConversation.bind(base),
    updateConversation: base.updateConversation.bind(base),
    removeConversation: base.removeConversation.bind(base),
    stopConversation: base.stopConversation.bind(base),
    streamChat(
      payload: AIChatChatRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      const contextItems = withZGISystemContextItems(getContextItems());
      const envelope = buildAIChatContextEnvelope(contextItems);
      const operationContext = buildAIChatOperationContext(contextItems);
      const mergedOperationContext = mergeAIChatOperationContext(
        operationContext,
        payload.operation_context
      );
      const wrappedCallbacks = wrapContextualCallbacks(callbacks, getContextItems, options);
      return aichatTransport.streamChat(
        {
          ...payload,
          surface: 'contextual_sidebar',
          runtime_context: envelope || undefined,
          operation_context: mergedOperationContext,
        },
        wrappedCallbacks,
        abortSignal
      );
    },
    regenerateMessage(
      messageId: string,
      payload: AIChatRegenerateMessageRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      const contextItems = withZGISystemContextItems(getContextItems());
      const envelope = buildAIChatContextEnvelope(contextItems);
      const operationContext = buildAIChatOperationContext(contextItems);
      const mergedOperationContext = mergeAIChatOperationContext(
        operationContext,
        payload.operation_context
      );
      return aichatTransport.regenerateMessage(
        messageId,
        {
          ...payload,
          surface: 'contextual_sidebar',
          runtime_context: envelope || payload.runtime_context,
          operation_context: mergedOperationContext,
        },
        wrapContextualCallbacks(callbacks, getContextItems, options),
        abortSignal
      );
    },
    recoverConversationStream(
      conversationId,
      params: { messageId: string; afterId?: string },
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      return aichatTransport.recoverConversationStream(
        conversationId,
        params,
        wrapContextualCallbacks(callbacks, getContextItems, options),
        abortSignal
      );
    },
    continueToolGovernanceDecision(
      conversationId: string,
      messageId: string,
      correlationId: string,
      payload: AIChatToolGovernanceDecisionRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      const wrappedCallbacks = wrapContextualCallbacks(callbacks, getContextItems, options);
      return aichatTransport.continueToolGovernanceDecision(
        conversationId,
        messageId,
        correlationId,
        payload,
        wrappedCallbacks,
        abortSignal
      );
    },
    continueClientAction(
      conversationId: string,
      messageId: string,
      actionId: string,
      payload: AIChatClientActionResultRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      const contextItems = withZGISystemContextItems(getContextItems());
      const envelope = buildAIChatContextEnvelope(contextItems);
      const operationContext = buildAIChatOperationContext(contextItems);
      const mergedOperationContext = mergeAIChatOperationContext(
        operationContext,
        payload.operation_context
      );
      return aichatTransport.continueClientAction(
        conversationId,
        messageId,
        actionId,
        {
          ...payload,
          runtime_context: envelope || payload.runtime_context,
          operation_context: mergedOperationContext,
        },
        wrapContextualCallbacks(callbacks, getContextItems, options),
        abortSignal
      );
    },
  };
}
