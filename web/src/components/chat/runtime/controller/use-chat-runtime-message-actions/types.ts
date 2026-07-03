import type { MutableRefObject } from 'react';

import type {
  AIChatControllerStore,
  AIChatSetControllerState,
} from '@/components/chat/controllers/aichat';
import type { AIChatRuntimeTransport } from '@/components/chat/transports/aichat-transport';
import type { ChatRuntimeEventAppliers } from '@/components/chat/runtime/controller/use-chat-runtime-event-appliers';
export interface UseChatRuntimeMessageActionsArgs {
  stateRef: MutableRefObject<AIChatControllerStore>;
  transportRef: MutableRefObject<AIChatRuntimeTransport>;
  requireModel: boolean;
  pendingStreamAbortRef: MutableRefObject<AbortController | null>;
  streamAbortByConversationRef: MutableRefObject<Record<string, AbortController>>;
  streamingMessageRef: MutableRefObject<{ conversationId: string; messageId: string } | null>;
  setControllerState: AIChatSetControllerState;
  markSelectionTarget: (conversationId: string | null) => number;
  isLatestSelection: (seq: number, conversationId: string | null) => boolean;
  refreshConversationSilently: (conversationId: string) => void;
  refreshMessagesSilently: (conversationId: string) => void;
  refreshAccountMemoryAfterMemoryMutation: (
    payload: Parameters<ChatRuntimeEventAppliers['applyMemoryMutation']>[0]
  ) => void;
  eventAppliers: ChatRuntimeEventAppliers;
}
