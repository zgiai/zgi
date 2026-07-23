export type ConversationRouteSyncAction =
  | { type: 'none' }
  | { type: 'replace'; conversationId: string }
  | { type: 'clear' };

export type ConversationRouteHandoff =
  | { conversationId: string; mode: 'selection' }
  | { conversationId: null; mode: 'new-chat' | 'draft-persistence' };

interface ConversationRouteSyncInput {
  activeConversationId: string | null;
  currentConversationId: string | null;
  routeHandoff: ConversationRouteHandoff | undefined;
  activeConversationIsDraft: boolean;
}

interface ConversationRouteSyncDecision {
  action: ConversationRouteSyncAction;
  routeHandoff: ConversationRouteHandoff | undefined;
}

export function shouldStartNewConversationForRoute(
  conversationIdParam: string | null,
  activeConversationId: string | null,
  activeConversationIsDraft: boolean
) {
  return !conversationIdParam && Boolean(activeConversationId) && !activeConversationIsDraft;
}

export function resolveConversationRouteSync({
  activeConversationId,
  currentConversationId,
  routeHandoff,
  activeConversationIsDraft,
}: ConversationRouteSyncInput): ConversationRouteSyncDecision {
  let nextRouteHandoff = routeHandoff;

  if (nextRouteHandoff !== undefined) {
    const targetConversationId = nextRouteHandoff.conversationId;
    if (activeConversationId === targetConversationId) {
      nextRouteHandoff = undefined;
    } else if (targetConversationId === null && activeConversationIsDraft) {
      return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
    } else if (targetConversationId === null && activeConversationId) {
      if (nextRouteHandoff.mode === 'new-chat') {
        return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
      }
      nextRouteHandoff = undefined;
    } else if (currentConversationId === targetConversationId) {
      return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
    }
  }

  if (activeConversationIsDraft) {
    return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
  }

  if (activeConversationId && currentConversationId !== activeConversationId) {
    if (currentConversationId) {
      return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
    }
    return {
      action: { type: 'replace', conversationId: activeConversationId },
      routeHandoff: nextRouteHandoff,
    };
  }

  if (!activeConversationId && currentConversationId) {
    return { action: { type: 'clear' }, routeHandoff: nextRouteHandoff };
  }

  return { action: { type: 'none' }, routeHandoff: nextRouteHandoff };
}
