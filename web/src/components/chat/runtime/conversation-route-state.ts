export function isConversationRouteRestoring(
  conversationIdParam: string | null,
  activeConversationId: string | null
) {
  const routeConversationId = conversationIdParam?.trim();
  if (!routeConversationId) return false;
  return routeConversationId !== activeConversationId?.trim();
}
