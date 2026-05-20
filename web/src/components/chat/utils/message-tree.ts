export interface ChatTreeNode {
  id: string;
  parent_id?: string;
  created_at: number;
}

export interface ChatConversationLeaf {
  current_leaf_message_id?: string;
}

export interface ChatBranchNavigation {
  current: number;
  total: number;
  previousId: string;
  nextId: string;
}

export interface ChatMessageTopology {
  sortedMessageIds: string[];
  parentById: Map<string, string | undefined>;
  childrenIdsByParent: Map<string, string[]>;
  messageIds: Set<string>;
}

/**
 * @util buildChatMessageTopologyKey
 * @description Creates a key that changes only when message tree topology changes.
 */
export function buildChatMessageTopologyKey<TMessage extends ChatTreeNode>(
  messages: TMessage[]
): string {
  return messages
    .map(message => `${message.id}:${message.parent_id?.trim() || ''}:${message.created_at}`)
    .join('|');
}

/**
 * @util buildChatMessageTopology
 * @description Builds reusable parent/children indexes for chat tree navigation.
 */
export function buildChatMessageTopology<TMessage extends ChatTreeNode>(
  messages: TMessage[]
): ChatMessageTopology {
  const sorted = [...messages].sort(
    (a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id)
  );
  const parentById = new Map<string, string | undefined>();
  const childrenIdsByParent = new Map<string, string[]>();
  const messageIds = new Set<string>();

  sorted.forEach(message => {
    const parentId = message.parent_id?.trim() || undefined;
    const parentKey = parentId || '';
    const children = childrenIdsByParent.get(parentKey) ?? [];

    messageIds.add(message.id);
    parentById.set(message.id, parentId);
    children.push(message.id);
    childrenIdsByParent.set(parentKey, children);
  });

  return {
    sortedMessageIds: sorted.map(message => message.id),
    parentById,
    childrenIdsByParent,
    messageIds,
  };
}

/**
 * @util buildChatMessageById
 * @description Indexes current message objects without rebuilding tree topology.
 */
export function buildChatMessageById<TMessage extends { id: string }>(
  messages: TMessage[]
): Map<string, TMessage> {
  return new Map(messages.map(message => [message.id, message]));
}

/**
 * @util getCurrentChatPathIds
 * @description Resolves the active branch path as message ids from a reusable topology index.
 */
export function getCurrentChatPathIds(
  conversation: ChatConversationLeaf | null | undefined,
  topology: ChatMessageTopology
): string[] {
  const leafId = conversation?.current_leaf_message_id?.trim();
  if (!leafId) return topology.sortedMessageIds;
  if (!topology.messageIds.has(leafId)) return topology.sortedMessageIds;

  let cursor: string | undefined = leafId;
  const visited = new Set<string>();
  const pathIds: string[] = [];

  while (cursor) {
    if (visited.has(cursor)) return topology.sortedMessageIds;
    if (!topology.messageIds.has(cursor)) {
      return pathIds.length > 0 ? pathIds.reverse() : topology.sortedMessageIds;
    }

    visited.add(cursor);
    pathIds.push(cursor);
    cursor = topology.parentById.get(cursor);
  }

  return pathIds.reverse();
}

/**
 * @util materializeChatMessages
 * @description Converts a path id list into current message objects.
 */
export function materializeChatMessages<TMessage>(
  messageIds: string[],
  messageById: Map<string, TMessage>
): TMessage[] {
  return messageIds
    .map(messageId => messageById.get(messageId))
    .filter((message): message is TMessage => Boolean(message));
}

export function buildCurrentChatPath<TMessage extends ChatTreeNode>(
  conversation: ChatConversationLeaf | null | undefined,
  messages: TMessage[]
): TMessage[] {
  const topology = buildChatMessageTopology(messages);
  const messageById = buildChatMessageById(messages);
  return materializeChatMessages(getCurrentChatPathIds(conversation, topology), messageById);
}

function getChatChildrenByParent<TMessage extends ChatTreeNode>(
  messages: TMessage[]
): Map<string, TMessage[]> {
  const childrenByParent = new Map<string, TMessage[]>();

  messages.forEach(message => {
    const parentKey = message.parent_id?.trim() || '';
    const children = childrenByParent.get(parentKey) ?? [];
    children.push(message);
    childrenByParent.set(parentKey, children);
  });

  childrenByParent.forEach(children => {
    children.sort((a, b) => a.created_at - b.created_at || a.id.localeCompare(b.id));
  });

  return childrenByParent;
}

export function getChatSiblingBranches<TMessage extends ChatTreeNode>(
  message: TMessage,
  messages: TMessage[]
): TMessage[] {
  const parentKey = message.parent_id?.trim() || '';
  return getChatChildrenByParent(messages).get(parentKey) ?? [];
}

export function buildChatBranchNavigationByMessageId(
  pathMessageIds: string[],
  topology: ChatMessageTopology
): Map<string, ChatBranchNavigation> {
  const navigation = new Map<string, ChatBranchNavigation>();

  pathMessageIds.forEach(messageId => {
    const parentKey = topology.parentById.get(messageId) || '';
    const siblings = topology.childrenIdsByParent.get(parentKey) ?? [];
    if (siblings.length <= 1) return;

    const currentIndex = siblings.findIndex(id => id === messageId);
    if (currentIndex < 0) return;

    navigation.set(messageId, {
      current: currentIndex + 1,
      total: siblings.length,
      previousId: siblings[(currentIndex - 1 + siblings.length) % siblings.length],
      nextId: siblings[(currentIndex + 1) % siblings.length],
    });
  });

  return navigation;
}

export function findChatBranchLeaf<TMessage extends ChatTreeNode>(
  messageId: string,
  messages: TMessage[]
): string {
  const byId = new Map(messages.map(message => [message.id, message]));
  const start = byId.get(messageId);
  if (!start) return messageId;

  const childrenByParent = getChatChildrenByParent(messages);
  let cursor = start;
  const visited = new Set<string>();

  while (!visited.has(cursor.id)) {
    visited.add(cursor.id);
    const children = childrenByParent.get(cursor.id) ?? [];
    if (children.length === 0) return cursor.id;
    cursor = children[children.length - 1];
  }

  return start.id;
}
