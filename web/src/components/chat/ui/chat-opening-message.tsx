import React from 'react';
import {
  ChatOpeningGuideView,
  type ChatOpeningGuideViewProps,
} from '@/components/chat/ui/chat-opening-guide-view';

interface ChatOpeningMessageProps extends Omit<ChatOpeningGuideViewProps, 'message'> {
  content: string;
}

/**
 * @component ChatOpeningMessage
 * @category UI
 * @status Stable
 * @description Backward-compatible wrapper for assistant-style opening messages.
 */
export function ChatOpeningMessage({ content, ...props }: ChatOpeningMessageProps) {
  return <ChatOpeningGuideView {...props} message={content} />;
}
