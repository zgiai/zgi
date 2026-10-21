import type { AIChatInitialConnectorBinding } from '@/services/types/aichat';
import type { ConnectorChatPreferences } from '@/services/types/connector';

export interface ChatConnectorDraft {
  id: string;
  scope: string;
  conversationId: string | null;
  revision: number;
  bindings: AIChatInitialConnectorBinding[];
  defaultsStatus: 'idle' | 'loading' | 'ready' | 'error';
  defaultsSealed: boolean;
  defaultsRevision?: number;
  defaultsSnapshot?: ConnectorChatPreferences;
  // Missing = inherit the captured snapshot; null = explicitly do not use.
  overrides: Record<string, string | null>;
}

export interface ChatConnectorDefaultsQuery {
  enabled: boolean;
  data?: ConnectorChatPreferences;
  isError?: boolean;
}

export function isUnsavedConnectorChat(id: string | null | undefined): boolean {
  return !id || id.startsWith('draft-');
}

export function newChatConnectorDraft(
  scope: string,
  conversationId: string | null
): ChatConnectorDraft {
  return {
    id: `draft-connectors-${crypto.randomUUID()}`,
    scope,
    conversationId,
    revision: 1,
    bindings: [],
    defaultsStatus: 'idle',
    defaultsSealed: false,
    overrides: {},
  };
}

function composeChatConnectorBindings(
  snapshot: ConnectorChatPreferences,
  overrides: ChatConnectorDraft['overrides']
): AIChatInitialConnectorBinding[] {
  const selected = new Map<string, string>();
  for (const [connectorID, choice] of Object.entries(snapshot.defaults)) {
    if (choice.mode === 'selected' && choice.connection_id) {
      selected.set(connectorID, choice.connection_id);
    }
  }
  for (const [connectorID, connectionID] of Object.entries(overrides)) {
    if (connectionID === null) selected.delete(connectorID);
    else selected.set(connectorID, connectionID);
  }
  return Array.from(selected, ([connector_id, connection_id]) => ({ connector_id, connection_id }));
}

function captureChatConnectorDraftDefaults(
  draft: ChatConnectorDraft,
  snapshot: ConnectorChatPreferences
): ChatConnectorDraft {
  return {
    ...draft,
    revision: draft.revision + 1,
    defaultsStatus: 'ready',
    defaultsRevision: snapshot.revision,
    defaultsSnapshot: snapshot,
    bindings: composeChatConnectorBindings(snapshot, draft.overrides),
  };
}

export function synchronizeChatConnectorDefaults(
  draft: ChatConnectorDraft,
  query?: ChatConnectorDefaultsQuery
): ChatConnectorDraft {
  if (
    !query?.enabled ||
    !isUnsavedConnectorChat(draft.conversationId) ||
    draft.defaultsSealed ||
    draft.defaultsRevision !== undefined
  ) {
    return draft;
  }
  if (query.isError) {
    return draft.defaultsStatus === 'error' ? draft : { ...draft, defaultsStatus: 'error' };
  }
  if (!query.data) {
    return draft.defaultsStatus === 'loading' ? draft : { ...draft, defaultsStatus: 'loading' };
  }
  return captureChatConnectorDraftDefaults(draft, query.data);
}

export function updateChatConnectorDraftBindings(
  draft: ChatConnectorDraft,
  bindings: AIChatInitialConnectorBinding[]
): ChatConnectorDraft {
  const overrides = { ...draft.overrides };
  const next = new Map(bindings.map(item => [item.connector_id, item.connection_id]));
  for (const binding of draft.bindings) {
    if (!next.has(binding.connector_id)) overrides[binding.connector_id] = null;
  }
  for (const [connectorID, connectionID] of next) {
    if (
      !draft.bindings.some(
        item => item.connector_id === connectorID && item.connection_id === connectionID
      )
    ) {
      overrides[connectorID] = connectionID;
    }
  }
  return {
    ...draft,
    revision: draft.revision + 1,
    defaultsSealed: true,
    overrides,
    bindings: Array.from(next, ([connector_id, connection_id]) => ({
      connector_id,
      connection_id,
    })),
  };
}

export function sealChatConnectorDraft(draft: ChatConnectorDraft): ChatConnectorDraft {
  return draft.defaultsSealed ? draft : { ...draft, defaultsSealed: true };
}

/** Local Connector attention only; preference reads never gate ordinary chat. */
export function chatConnectorDraftAttention(
  draft: ChatConnectorDraft
): 'loading' | 'loadFailed' | 'unavailable' | null {
  if (draft.defaultsStatus === 'loading') return 'loading';
  if (draft.defaultsStatus === 'error') return 'loadFailed';
  const defaults = draft.defaultsSnapshot?.defaults ?? {};
  return draft.bindings.some(binding => {
    const choice = defaults[binding.connector_id];
    return (
      choice?.mode === 'selected' &&
      choice.connection_id === binding.connection_id &&
      choice.available !== true
    );
  })
    ? 'unavailable'
    : null;
}

export function reconcileChatConnectorDraft(
  draft: ChatConnectorDraft,
  scope: string,
  conversationId: string | null
): ChatConnectorDraft {
  if (draft.scope === scope && draft.conversationId === conversationId) return draft;
  if (
    draft.scope !== scope ||
    !isUnsavedConnectorChat(conversationId) ||
    (!conversationId && draft.conversationId)
  ) {
    return newChatConnectorDraft(scope, conversationId);
  }
  // null -> sending draft (and retries within that draft) keep the same choice.
  return { ...draft, conversationId };
}

export function selectChatDraftConnection(
  draft: ChatConnectorDraft,
  binding: AIChatInitialConnectorBinding
): AIChatInitialConnectorBinding[] {
  return [
    ...draft.bindings.filter(item => item.connector_id !== binding.connector_id),
    {
      connector_id: binding.connector_id,
      connection_id: binding.connection_id,
    },
  ];
}
