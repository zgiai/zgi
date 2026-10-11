export const MAX_QUEUED_CHAT_TURNS = 2;

export interface AIChatQueuedTurn {
  id: string;
  content: string;
  useMemory: boolean;
  createdAt: number;
}

export function enqueueQueuedChatTurn(
  turns: AIChatQueuedTurn[],
  turn: AIChatQueuedTurn
): AIChatQueuedTurn[] {
  if (turns.length >= MAX_QUEUED_CHAT_TURNS) return turns;
  if (turns.some(item => item.id === turn.id)) return turns;
  return [...turns, turn].sort((left, right) => left.createdAt - right.createdAt);
}

export function removeQueuedChatTurn(
  turns: AIChatQueuedTurn[],
  turnID: string
): AIChatQueuedTurn[] {
  return turns.filter(turn => turn.id !== turnID);
}
