import React, { useEffect, useMemo, useRef } from 'react';
import type { Conversation, NodeRunStatus } from '@/components/chat/types';
import MessageItem from '@/components/chat/ui/message-item';
import WorkflowRunHeader from '@/components/chat/ui/workflow-run-header';
import { ChatHomeView } from '@/components/chat/ui/chat-home-view';
import { ChatOpeningMessage } from '@/components/chat/ui/chat-opening-message';
import { Skeleton } from '@/components/ui/skeleton';
import { eventBus } from '@/lib/event-bus';
import { cn } from '@/lib/utils';
import type { OpeningGuideConfig } from '@/utils/webapp/opening-statement';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';

interface ConversationBoxProps {
  conversation: Conversation | undefined;
  renderHeader?: (c: Conversation) => React.ReactNode;
  className?: string;
  /** Show expanded input/output details in workflow node list */
  showWorkflowNodeDetail?: boolean;
  /** Show workflow run header with node progress */
  showWorkflowRunHeader?: boolean;
  /** Show workflow run monitor in message item (hide entire workflow section if false) */
  showWorkflowDetail?: boolean;
  /** Hide completed workflow detail cards. Webapp uses this to keep completed chats clean. */
  hideCompletedWorkflowDetail?: boolean;
  /** Allow expanding workflow run summary into node details */
  allowWorkflowDetailExpand?: boolean;
  /** Default open state for workflow run summary */
  defaultWorkflowDetailOpen?: boolean;
  /** Show loading skeleton when conversation is loading */
  isLoading?: boolean;
  /** Callback when suggestion is clicked in home view */
  onSuggestionClick?: (text: string) => void;
  /** Optional workflow-configured opening guide to show for empty conversations */
  openingGuide?: OpeningGuideConfig;
  suggestions?: string[];
  suggestionsTitle?: string;
  showDefaultSuggestions?: boolean;
  renderMessageAddon?: (message: Conversation['messages'][number]) => React.ReactNode;
}

const ConversationBox: React.FC<ConversationBoxProps> = ({
  conversation,
  renderHeader,
  className,
  showWorkflowNodeDetail = false,
  showWorkflowRunHeader = false,
  showWorkflowDetail = true,
  hideCompletedWorkflowDetail = false,
  allowWorkflowDetailExpand = true,
  defaultWorkflowDetailOpen = true,
  isLoading = false,
  onSuggestionClick,
  openingGuide,
  suggestions,
  suggestionsTitle,
  showDefaultSuggestions = false,
  renderMessageAddon,
}) => {
  const scrollRef = useRef<HTMLDivElement | null>(null);
  const bottomRef = useRef<HTMLDivElement | null>(null);
  const messages = useMemo(() => conversation?.messages ?? [], [conversation]);
  const normalizedOpeningGuide = useMemo(() => {
    if (!openingGuide) return undefined;

    if (openingGuide.type === 'slogan') {
      const slogan = typeof openingGuide.slogan === 'string' ? openingGuide.slogan.trim() : '';
      return slogan
        ? {
            type: 'slogan' as const,
            slogan,
          }
        : undefined;
    }

    const message = typeof openingGuide.message === 'string' ? openingGuide.message : '';
    return message.trim()
      ? {
          type: 'message' as const,
          message,
        }
      : undefined;
  }, [openingGuide]);
  const normalizedSuggestions = useMemo(
    () =>
      (suggestions ?? [])
        .map(item => item.trim())
        .filter(Boolean)
        .slice(0, SUGGESTED_QUESTIONS_LIMIT),
    [suggestions]
  );
  const autoFollowRef = useRef(true);
  const activeTempKeyRef = useRef<string | null>(null);
  const rafRef = useRef<number | null>(null);

  const scrollToBottom = useMemo(
    () =>
      (behavior: ScrollBehavior = 'auto') => {
        const el = scrollRef.current;
        if (!el) return;

        el.scrollTo({
          top: el.scrollHeight,
          behavior,
        });
      },
    []
  );

  // Get the currently running workflow from the last message
  const currentWorkflowRun = useMemo(() => {
    if (!messages.length) return null;
    const lastMessage = messages[messages.length - 1];
    const workflowInfo = lastMessage?.WorkflowRunInfo;
    if (!workflowInfo) return null;
    if (
      hideCompletedWorkflowDetail &&
      (workflowInfo.status === 'completed' ||
        (workflowInfo.status as string | undefined) === 'normal')
    ) {
      return null;
    }

    const mapStatus = (
      s: NodeRunStatus
    ): 'running' | 'succeeded' | 'failed' | 'stopped' | 'paused' => {
      if (s === 'failed') return 'failed';
      if (s === 'stopped') return 'stopped';
      if (s === 'paused') return 'paused';
      if (s === 'success' || s === 'succeeded') return 'succeeded';
      return 'running';
    };

    const nodeItems = (workflowInfo.runNodeInfo ?? []).map((n, idx) => {
      const base = {
        title: n.title || `Step ${idx + 1}`,
        nodeId: n.nodeId || `step-${idx}`,
        nodeType: n.nodeType || 'unknown',
        status: mapStatus(n.status),
        nodeInput: n.data?.input ?? undefined,
        nodeOutput: n.data?.output ?? undefined,
        modelInput: n.data?.modelInput ?? undefined,
        elapsedTime: n.elapsedTime,
        error: n.error ?? null,
      };
      if (n.nodeType === 'iteration' || n.nodeType === 'loop') {
        const roundsSource =
          n.nodeType === 'loop' ? (n.loopRounds ?? []) : (n.iterationRounds ?? []);
        const rounds = roundsSource.map(r => ({
          index: r.index,
          elapsedTime: r.elapsedTime,
          nodes: (r.nodes ?? []).map((child, cidx) => ({
            title: child.title || `Step ${cidx + 1}`,
            nodeId: child.nodeId || `step-${cidx}`,
            nodeType: child.nodeType || 'unknown',
            status: mapStatus(child.status),
            nodeInput: child.data?.input ?? undefined,
            nodeOutput: child.data?.output ?? undefined,
            modelInput: child.data?.modelInput ?? undefined,
            elapsedTime: child.elapsedTime,
            error: child.error ?? null,
          })),
        }));
        return {
          ...base,
          iterationInputs: n.nodeType === 'iteration' ? n.iterationInputs : undefined,
          iterationOutputs: n.nodeType === 'iteration' ? n.iterationOutputs : undefined,
          iterationRounds: n.nodeType === 'iteration' ? rounds : undefined,
          loopInputs: n.nodeType === 'loop' ? n.loopInputs : undefined,
          loopOutputs: n.nodeType === 'loop' ? n.loopOutputs : undefined,
          loopRounds: n.nodeType === 'loop' ? rounds : undefined,
          steps: typeof n.steps === 'number' ? n.steps : undefined,
        };
      }
      return base;
    });

    return {
      status: workflowInfo.status,
      items: nodeItems,
    };
  }, [hideCompletedWorkflowDetail, messages]);

  useEffect(() => {
    // Auto scroll to anchor when messages change
    if (autoFollowRef.current) {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          scrollToBottom();
        });
      });
    }
  }, [messages.length, scrollToBottom]);

  // Distance from bottom threshold to consider "at bottom"
  const NEAR_BOTTOM_THRESHOLD = 50;

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const handleScroll = () => {
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      if (distanceFromBottom > NEAR_BOTTOM_THRESHOLD) {
        // User scrolled up, disable auto-follow
        autoFollowRef.current = false;
      } else {
        // User scrolled back to bottom, restore auto-follow
        autoFollowRef.current = true;
      }
    };
    el.addEventListener('scroll', handleScroll, { passive: true });
    return () => el.removeEventListener('scroll', handleScroll);
  }, []);

  useEffect(() => {
    const unsub = eventBus.subscribe<{ conversationId: string; type: string; tempKey?: string }>(
      'chat:scroll',
      payload => {
        if (!conversation) return;
        if (payload.conversationId !== conversation.id) return;

        // Reset autoFollow when a new run starts
        if (payload.type === 'workflow_started') {
          autoFollowRef.current = true;
          activeTempKeyRef.current = payload.tempKey ?? null;
        }

        // Skip scroll if user has scrolled up (auto-follow disabled)
        if (!autoFollowRef.current) return;

        // Perform scroll for all event types when auto-follow is enabled
        const el = scrollRef.current;
        if (!el) return;
        if (rafRef.current) cancelAnimationFrame(rafRef.current);
        // Double rAF to wait for React commit + layout
        rafRef.current = requestAnimationFrame(() => {
          rafRef.current = requestAnimationFrame(() => {
            scrollToBottom();
          });
        });

        if (payload.type === 'workflow_finished') {
          activeTempKeyRef.current = null;
        }
      }
    );
    return () => {
      unsub();
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [conversation, scrollToBottom]);

  // Render header even when loading or no conversation
  const headerNode = useMemo(() => {
    if (!renderHeader) return null;
    // Pass a fallback conversation for header rendering when loading
    const convForHeader = conversation ?? {
      id: '',
      conversationId: '',
      title: '',
      messages: [],
      conversationData: {},
    };
    return <div>{renderHeader(convForHeader)}</div>;
  }, [renderHeader, conversation]);

  // Loading state
  if (isLoading) {
    return (
      <div className={cn('flex min-h-0 flex-col overflow-hidden', className)}>
        {headerNode}
        <div className="flex flex-col gap-4 p-4">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      </div>
    );
  }

  if (!conversation) {
    return (
      <div className={cn('flex min-h-0 flex-col overflow-hidden', className)}>
        {headerNode}
        <div className="p-4 text-sm text-muted-foreground">暂无会话</div>
      </div>
    );
  }

  return (
    <div className={cn('flex min-h-0 flex-col overflow-hidden', className)}>
      {headerNode}
      {showWorkflowRunHeader && currentWorkflowRun && (
        <WorkflowRunHeader
          status={currentWorkflowRun.status}
          items={currentWorkflowRun.items}
          visible
        />
      )}
      <div className="h-0 min-h-0 w-full grow overflow-hidden">
        <div ref={scrollRef} className="h-full min-w-0 overflow-x-hidden overflow-y-auto p-3">
          {messages.length === 0 ? (
            normalizedOpeningGuide ? (
              normalizedOpeningGuide.type === 'slogan' ? (
                <div className="mx-auto flex h-full w-full min-w-0 max-w-6xl overflow-hidden">
                  <ChatHomeView
                    className="max-w-none"
                    title={normalizedOpeningGuide.slogan}
                    onSuggestionClick={onSuggestionClick}
                    suggestions={normalizedSuggestions}
                    suggestionsTitle={suggestionsTitle}
                    showDefaultSuggestions={showDefaultSuggestions}
                  />
                  <div ref={bottomRef} />
                </div>
              ) : (
                <div className="mx-auto w-full min-w-0 max-w-6xl space-y-6 pt-4">
                  <ChatOpeningMessage content={normalizedOpeningGuide.message} />
                  <ChatHomeView
                    className="h-auto justify-start px-0 py-0"
                    title=""
                    suggestions={normalizedSuggestions}
                    suggestionsTitle={suggestionsTitle}
                    showDefaultSuggestions={showDefaultSuggestions}
                    onSuggestionClick={onSuggestionClick}
                  />
                  <div ref={bottomRef} />
                </div>
              )
            ) : (
              <ChatHomeView
                onSuggestionClick={onSuggestionClick}
                suggestions={normalizedSuggestions}
                suggestionsTitle={suggestionsTitle}
                showDefaultSuggestions={showDefaultSuggestions}
              />
            )
          ) : (
            <div className="mx-auto max-w-6xl space-y-6">
              {messages.map((m, idx) => {
                const workflowStatus = m.WorkflowRunInfo?.status;
                const shouldShowWorkflowDetail =
                  showWorkflowDetail &&
                  (!hideCompletedWorkflowDetail ||
                    (workflowStatus !== 'completed' &&
                      (workflowStatus as string | undefined) !== 'normal'));
                return (
                  <MessageItem
                    key={(m.messageData?.tempKey as string) ?? `${idx}`}
                    message={m}
                    messageAddon={renderMessageAddon?.(m)}
                    showWorkflowNodeDetail={showWorkflowNodeDetail}
                    showWorkflowDetail={shouldShowWorkflowDetail}
                    allowWorkflowDetailExpand={allowWorkflowDetailExpand}
                    defaultWorkflowDetailOpen={defaultWorkflowDetailOpen}
                  />
                );
              })}
              <div ref={bottomRef} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default ConversationBox;
