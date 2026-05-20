import React from 'react';
import { Bot } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { cn } from '@/lib/utils';

interface ChatOpeningMessageProps {
  className?: string;
  content: string;
}

/**
 * @component ChatOpeningMessage
 * @category UI
 * @status Stable
 * @description Renders a workflow-configured opening statement as an assistant-style empty state.
 * @usage Use in chat containers when a conversation has no messages and a workflow defines an opening statement.
 * @example
 * <ChatOpeningMessage content="## Hello\nHow can I help today?" />
 */
export function ChatOpeningMessage({ className, content }: ChatOpeningMessageProps) {
  const normalizedContent = content.trim();

  if (!normalizedContent) {
    return null;
  }

  return (
    <div className={cn('max-w-full min-w-0 space-y-3 overflow-hidden', className)}>
      <div className="flex min-w-0 justify-start">
        <div className="w-full max-w-full min-w-0 overflow-hidden text-sm text-foreground">
          <div className="flex items-center">
            <div className="h-7 w-7 rounded-full bg-primary flex items-center justify-center">
              <Bot size={20} className="text-primary-foreground" />
            </div>
          </div>
          <div className="mt-2 max-w-full min-w-0 overflow-x-hidden">
            <MarkdownViewer
              className="md-viewer max-w-full break-words [&_pre]:max-w-full [&_pre]:overflow-x-auto [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto"
              content={normalizedContent}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
