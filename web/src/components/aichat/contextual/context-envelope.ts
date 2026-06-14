import type {
  AIChatRuntimeTransport,
  AIChatStreamCallbacks,
} from '@/components/chat/transports/aichat-transport';
import { aichatTransport } from '@/components/chat/transports/aichat-transport';
import type {
  AIChatChatRequest,
  AIChatRegenerateMessageRequest,
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

const RISK_RANK: Record<AIChatCapabilityRisk, number> = {
  low: 1,
  medium: 2,
  high: 3,
};

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
    'Current ZGI page context. Use it only to interpret this turn; do not save it as memory unless the user explicitly asks.',
    'Important: AIChat account memory and Agent memory are separate. Do not claim they are shared.',
    'Important: Workflow resources and Agent resources are distinct; a workflow binding does not make the workflow the Agent.',
  ];

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

export function createContextualAIChatTransport(
  getContextItems: () => AIChatContextItem[]
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
      const contextItems = getContextItems();
      const envelope = buildAIChatContextEnvelope(contextItems);
      const operationContext = buildAIChatOperationContext(contextItems);
      return aichatTransport.streamChat(
        {
          ...payload,
          runtime_context: envelope || undefined,
          operation_context:
            operationContext.resources.length > 0 ? operationContext : payload.operation_context,
        },
        callbacks,
        abortSignal
      );
    },
    regenerateMessage(
      messageId: string,
      payload: AIChatRegenerateMessageRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      return aichatTransport.regenerateMessage(messageId, payload, callbacks, abortSignal);
    },
    recoverConversationStream: base.recoverConversationStream.bind(base),
    continueToolGovernanceDecision(
      conversationId: string,
      messageId: string,
      correlationId: string,
      payload: AIChatToolGovernanceDecisionRequest,
      callbacks: AIChatStreamCallbacks,
      abortSignal?: AbortSignal
    ) {
      return aichatTransport.continueToolGovernanceDecision(
        conversationId,
        messageId,
        correlationId,
        payload,
        callbacks,
        abortSignal
      );
    },
  };
}
