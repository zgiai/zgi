import { createStore, type StoreApi } from 'zustand/vanilla';
import { queryClient } from '@/lib/query-client';
import { createTextStreamThrottler } from '@/utils/throttle-text-stream';
import { STREAM_RENDER_THROTTLE_MS } from '@/lib/config';
import type {
  ChatController,
  ConversationTransport,
  ConversationSummary,
  ConversationSearchResult,
  ConversationDetail,
  Pagination,
  SendMessagePayload,
  ChatMode,
} from './types';
import { useChatStore } from '@/components/chat/store';
import type {
  ModelInfo,
  NodeInfo,
  GeneratedImage,
  TerminalRunStatus,
} from '@/components/chat/types';
import { SENSITIVE_OUTPUT_BLOCKED_TOKEN } from '@/utils/model-output-filter';
import { resolveAnswerMergeMode } from '@/components/chat/utils/answer-merge';

interface SingleChatControllerState {
  mode: ChatMode;
  transport: ConversationTransport;
  conversations: ConversationSummary[];
  pagination: Pagination;
  activeId: string | null;
  activeDetail: ConversationDetail | null;
  isLoadingList: boolean;
  isLoadingDetail: boolean;
  isSending: boolean;
  isPaused: boolean;
  initialized: boolean;
  lastInputs?: Record<string, unknown>;
}

interface SingleChatControllerStore extends SingleChatControllerState {
  setTransport: (transport: ConversationTransport) => void;
  setConversations: (conversations: ConversationSummary[]) => void;
  setPagination: (pagination: Pagination) => void;
  setActiveId: (id: string | null) => void;
  setActiveDetail: (detail: ConversationDetail | null) => void;
  setIsLoadingList: (loading: boolean) => void;
  setIsLoadingDetail: (loading: boolean) => void;
  setIsSending: (sending: boolean) => void;
  setIsPaused: (paused: boolean) => void;
  setInitialized: (initialized: boolean) => void;
  setLastInputs: (inputs: Record<string, unknown> | undefined) => void;

  // Adopt server conversation id
  adoptServerConversationId: (clientId: string, serverConversationId: string) => void;
}

const createControllerStore = () =>
  createStore<SingleChatControllerStore>()(set => ({
    mode: 'singleChat',
    transport: null as unknown as ConversationTransport,
    conversations: [],
    pagination: { page: 1, limit: 20, total: 0, hasMore: false },
    activeId: null,
    activeDetail: null,
    isLoadingList: false,
    isLoadingDetail: false,
    isSending: false,
    isPaused: false,
    initialized: false,
    lastInputs: undefined,

    setTransport: transport => set({ transport }),
    setConversations: conversations => set({ conversations }),
    setPagination: pagination => set({ pagination }),
    setActiveId: id => set({ activeId: id }),
    setActiveDetail: detail => set({ activeDetail: detail }),
    setIsLoadingList: loading => set({ isLoadingList: loading }),
    setIsLoadingDetail: loading => set({ isLoadingDetail: loading }),
    setIsSending: sending => set({ isSending: sending }),
    setIsPaused: paused => set({ isPaused: paused }),
    setInitialized: initialized => set({ initialized }),
    setLastInputs: inputs => set({ lastInputs: inputs }),

    adoptServerConversationId: (clientId, serverConversationId) => {
      set(state => {
        // 1. Update conversation list: replace draft id with server id
        const nextConversations = state.conversations.map(c =>
          c.id === clientId
            ? { ...c, id: serverConversationId, conversationId: serverConversationId }
            : c
        );

        // 2. Update active id if it was the draft
        const nextActiveId = state.activeId === clientId ? serverConversationId : state.activeId;

        // 3. Update active detail if it matches
        const nextActiveDetail =
          state.activeDetail?.summary.id === clientId
            ? {
                ...state.activeDetail,
                summary: {
                  ...state.activeDetail.summary,
                  id: serverConversationId,
                  conversationId: serverConversationId,
                },
              }
            : state.activeDetail;

        return {
          conversations: nextConversations,
          activeId: nextActiveId,
          activeDetail: nextActiveDetail,
        };
      });

      // 4. Migrate chat store data
      useChatStore.getState().migrateConversation(clientId, serverConversationId);
    },
  }));

export class SingleChatController implements ChatController {
  private transport: ConversationTransport;
  public store: StoreApi<SingleChatControllerStore>;
  private titleRefreshTimers = new Set<ReturnType<typeof setTimeout>>();

  readonly mode = 'singleChat' as const;

  constructor(transport: ConversationTransport) {
    this.transport = transport;
    this.store = createControllerStore();
    // Note: setTransport must be called via initTransport() in useEffect to avoid setState during render
  }

  // Initialize transport in store - call this from useEffect, not during render
  initTransport(): void {
    this.store.getState().setTransport(this.transport);
  }

  updateTransport(transport: ConversationTransport): void {
    this.transport = transport;
    this.store.getState().setTransport(transport);
  }

  private conversationTitleNeedsRefresh(title?: string): boolean {
    const normalized = (title ?? '').trim();
    if (!normalized) return true;
    if (normalized === 'New Conversation') return true;
    if (normalized.startsWith('New conversation ')) return true;
    if (/^Conversation \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(normalized)) {
      return true;
    }
    return false;
  }

  private applyConversationSnapshot(detail: ConversationDetail): { needsRefresh: boolean } {
    const summary = detail.summary;
    const conversations = this.store.getState().conversations;
    const current = conversations.find(
      c => c.id === summary.id || c.conversationId === summary.conversationId
    );
    const nextConversations = current
      ? conversations.map(c =>
          c.id === current.id
            ? {
                ...c,
                ...summary,
                id: current.id,
                conversationId: summary.conversationId || c.conversationId,
              }
            : c
        )
      : [summary, ...conversations];

    this.store
      .getState()
      .setConversations(nextConversations.sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0)));

    const activeDetail = this.store.getState().activeDetail;
    if (
      activeDetail?.summary.id === summary.id ||
      activeDetail?.summary.conversationId === summary.conversationId
    ) {
      this.store.getState().setActiveDetail({
        ...activeDetail,
        summary: {
          ...activeDetail.summary,
          ...summary,
          id: activeDetail.summary.id,
          conversationId: summary.conversationId || activeDetail.summary.conversationId,
        },
      });
    }

    useChatStore.getState().initSingle({
      id: current?.id ?? summary.id,
      conversationId: summary.conversationId,
      title: summary.title,
    });

    return { needsRefresh: this.conversationTitleNeedsRefresh(summary.title) };
  }

  private async refreshConversationSnapshotSilently(
    conversationId: string
  ): Promise<{ needsRefresh: boolean }> {
    if (!conversationId || conversationId.startsWith('draft-')) {
      return { needsRefresh: false };
    }
    try {
      const detail = await this.transport.get(conversationId);
      return this.applyConversationSnapshot(detail);
    } catch (err) {
      console.error('[SingleChatController] Failed to refresh conversation snapshot:', err);
      return { needsRefresh: true };
    }
  }

  private scheduleConversationTitleRefresh(conversationId: string): void {
    if (!conversationId || conversationId.startsWith('draft-')) return;

    const delays = [750, 2500, 6000, 12000];
    const run = (index: number) => {
      const timer = setTimeout(() => {
        this.titleRefreshTimers.delete(timer);
        void this.refreshConversationSnapshotSilently(conversationId).then(result => {
          if (result.needsRefresh && index + 1 < delays.length) {
            run(index + 1);
          }
        });
      }, delays[index]);
      this.titleRefreshTimers.add(timer);
    };

    run(0);
  }

  // State getters (reactive via zustand)
  get conversations(): ConversationSummary[] {
    return this.store.getState().conversations;
  }

  get pagination(): Pagination {
    return this.store.getState().pagination;
  }

  get activeId(): string | null {
    return this.store.getState().activeId;
  }

  get activeDetail(): ConversationDetail | null {
    return this.store.getState().activeDetail;
  }

  get isLoadingList(): boolean {
    return this.store.getState().isLoadingList;
  }

  get isLoadingDetail(): boolean {
    return this.store.getState().isLoadingDetail;
  }

  get isSending(): boolean {
    return this.store.getState().isSending;
  }

  get isPaused(): boolean {
    return this.store.getState().isPaused;
  }

  // Actions
  init(convId?: string): void {
    const { initialized } = this.store.getState();

    // If a convId is provided, set it as active immediately
    if (convId) {
      this.store.getState().setActiveId(convId);

      // If already initialized, we can try to load/select it directly
      if (initialized) {
        this.loadAndSelect(convId);
        return;
      }
    } else if (initialized) {
      return;
    }

    this.store.getState().setInitialized(true);
    this.refreshList({ page: 1, limit: 20 }).then(() => {
      // If convId was provided, ensure it's loaded and selected after list refresh.
      // We call loadAndSelect unconditionally here because:
      // 1. If it's in the list (loaded by refreshList), loadAndSelect will delegate to select()
      // 2. If it's NOT in the list, loadAndSelect will fetch it
      if (convId) {
        this.loadAndSelect(convId);
      }
    });
  }

  async loadAndSelect(conversationId: string): Promise<void> {
    // Check if already in list
    const existing = this.store.getState().conversations.find(c => c.id === conversationId);
    if (existing) {
      this.select(conversationId);
      return;
    }

    this.store.getState().setIsLoadingDetail(true);
    try {
      // We assume conversationId is a server ID here.
      // If transport supports get(), we can fetch detail directly.
      const detail = await this.transport.get(conversationId);

      // Add to list if not present
      const summary: ConversationSummary = detail.summary;

      // Update list
      const currentList = this.store.getState().conversations;
      if (!currentList.find(c => c.id === summary.id)) {
        this.store.getState().setConversations([summary, ...currentList]);
      }

      // Select it
      this.store.getState().setActiveId(summary.id);

      // Initialize chat store
      useChatStore.getState().initSingle({
        id: detail.summary.id,
        conversationId: detail.summary.conversationId,
        title: detail.summary.title,
        messages: detail.messages,
      });

      this.store.getState().setActiveDetail(detail);
    } catch (err) {
      console.error(`[SingleChatController] Failed to load conversation ${conversationId}:`, err);
      // Fallback: clear activeId (return to home view) and do NOT show toast
      this.store.getState().setActiveId(null);
    } finally {
      this.store.getState().setIsLoadingDetail(false);
    }
  }

  async refreshList(params?: { page?: number; limit?: number; append?: boolean }): Promise<void> {
    const { page = 1, limit = 20, append = false } = params ?? {};
    this.store.getState().setIsLoadingList(true);

    try {
      const result = await this.transport.list({ page, limit });
      const existing = this.store.getState().conversations;

      // Filter out drafts from existing list to preserve them
      const drafts = existing.filter(
        c => !c.conversationId || c.conversationId.trim().length === 0
      );

      if (append) {
        const existingIds = new Set(existing.map(c => c.id));
        const toAppend = result.items.filter(c => !existingIds.has(c.id));
        this.store.getState().setConversations([...existing, ...toAppend]);
      } else {
        // If not appending (refreshing), we keep drafts at the top and replace the rest with server items
        // Also filter out any server items that might conflict with drafts (though unlikely as drafts use client IDs)
        const serverItemIds = new Set(result.items.map(c => c.id));
        const filteredDrafts = drafts.filter(d => !serverItemIds.has(d.id));
        const merged = [...filteredDrafts, ...result.items];
        this.store.getState().setConversations(merged);
      }
      this.store.getState().setPagination(result.pagination);

      // Do NOT create draft if list is empty - let UI handle empty state
      if (result.items.length === 0) {
        // Only clear activeId if we don't have a draft selected
        const currentActiveId = this.store.getState().activeId;
        const isDraftActive = currentActiveId && drafts.some(d => d.id === currentActiveId);

        if (!isDraftActive) {
          this.store.getState().setActiveId(null);
        }
      }
    } catch (err) {
      // Error toast handled in transport hook
      console.error('[SingleChatController] Failed to refresh list:', err);
    } finally {
      this.store.getState().setIsLoadingList(false);
    }
  }

  async select(id: string): Promise<void> {
    this.store.getState().setActiveId(id);

    const conv = this.store.getState().conversations.find(c => c.id === id);
    if (!conv) return;

    // If conversation has no backend id, it's a draft, skip loading detail
    if (!conv.conversationId || conv.conversationId.trim().length === 0) {
      // Initialize chat store with empty conversation
      useChatStore.getState().initSingle({ id: conv.id, conversationId: '', title: conv.title });
      this.store.getState().setActiveDetail({
        summary: conv,
        messages: [],
        loaded: true,
        loading: false,
      });
      return;
    }

    // Load detail using TanStack Query caching keyed by conversationId
    this.store.getState().setIsLoadingDetail(true);
    try {
      const detail = await queryClient.fetchQuery({
        queryKey: ['conversation-detail', conv.conversationId],
        queryFn: () => this.transport.get(conv.conversationId),
        staleTime: 30 * 1000,
        gcTime: 5 * 60 * 1000,
        retry: false,
      });

      // Initialize chat store with loaded messages
      useChatStore.getState().initSingle({
        id: detail.summary.id,
        conversationId: detail.summary.conversationId,
        title: detail.summary.title,
        messages: detail.messages,
      });

      this.store.getState().setActiveDetail(detail);
    } catch (err) {
      // Error toast handled in transport hook
      console.error('[SingleChatController] Failed to load detail:', err);
    } finally {
      this.store.getState().setIsLoadingDetail(false);
    }
  }

  createDraft(title?: string): ConversationSummary {
    // Check if there is already a draft
    const existingDraft = this.store
      .getState()
      .conversations.find(c => !c.conversationId || c.conversationId.trim().length === 0);

    if (existingDraft) {
      // If active is not the draft, switch to it
      if (this.store.getState().activeId !== existingDraft.id) {
        this.select(existingDraft.id);
      } else {
        // ... (toast logic)
      }
      return existingDraft;
    }

    const draft: ConversationSummary = {
      id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
      conversationId: '',
      title: title ?? 'New Conversation',
      dialogueCount: 0,
      updatedAt: Date.now(),
      status: 'draft',
    };

    // Prepend to list
    this.store.getState().setConversations([draft, ...this.store.getState().conversations]);

    // Initialize in chat store
    useChatStore.getState().initSingle({ id: draft.id, conversationId: '', title: draft.title });

    return draft;
  }

  async remove(id: string): Promise<void> {
    const conv = this.store.getState().conversations.find(c => c.id === id);
    if (!conv) return;

    try {
      await this.transport.remove(conv.conversationId);

      // Remove from list
      const nextConversations = this.store.getState().conversations.filter(c => c.id !== id);
      this.store.getState().setConversations(nextConversations);

      // If was active, select another
      if (this.store.getState().activeId === id) {
        if (nextConversations.length > 0) {
          this.select(nextConversations[0].id);
        } else {
          // If list is empty, clear active id (show empty state)
          this.store.getState().setActiveId(null);
        }
      }

      // Delete from chat store
      useChatStore.getState().deleteConversation(id);
    } catch (err) {
      console.error('[SingleChatController] Failed to remove:', err);
    }
  }

  send(payload: Omit<SendMessagePayload, 'conversationId'> & { conversationId?: string }): void {
    let activeId = payload.conversationId ?? this.store.getState().activeId;

    // If no active conversation, create a draft first
    if (!activeId) {
      const draft = this.createDraft();
      this.select(draft.id);
      activeId = draft.id;
    }

    if (!activeId) {
      console.warn('[SingleChatController] No active conversation to send to');
      return;
    }

    const conv = this.store.getState().conversations.find(c => c.id === activeId);
    if (!conv) {
      console.warn('[SingleChatController] Active conversation not found in list');
      return;
    }

    this.store.getState().setIsSending(true);
    this.store.getState().setIsPaused(false);
    // Persist last inputs
    this.store.getState().setLastInputs(payload.inputs);

    const existingMessages = useChatStore.getState().conversations[activeId]?.messages ?? [];
    const latestMessage = existingMessages[existingMessages.length - 1];
    const isQuestionAnswerResume =
      latestMessage?.WorkflowRunInfo?.status === 'pending_question' ||
      latestMessage?.clientState?.status === 'pending_question';
    const resumeConversationId =
      isQuestionAnswerResume && typeof latestMessage?.messageData?.conversation_id === 'string'
        ? latestMessage.messageData.conversation_id.trim()
        : '';
    const requestConversationId =
      payload.conversationId ?? (conv.conversationId || resumeConversationId);
    const existingTempKey =
      typeof latestMessage?.messageData?.tempKey === 'string'
        ? (latestMessage.messageData.tempKey as string)
        : '';
    const tempKey =
      isQuestionAnswerResume && existingTempKey
        ? existingTempKey
        : useChatStore.getState().appendUserMessage(activeId, { query: payload.query }).tempKey;
    useChatStore.getState().ensureAiMessage(activeId, tempKey);

    // Use a mutable reference for the current conversation ID
    // This allows us to handle ID migration (draft -> server) during the stream
    let currentId = activeId;

    // Create a throttler for text chunks to implement a smooth typewriter effect
    // and reduce React re-render overhead during high-frequency SSE events.
    const textThrottler = createTextStreamThrottler(STREAM_RENDER_THROTTLE_MS, (text: string) => {
      useChatStore.getState().ensureAiMessage(currentId, tempKey);
      useChatStore.getState().appendAiChunk(currentId, tempKey, text);
    });

    const replaceWithBlockedAnswer = () => {
      textThrottler.cancel();
      useChatStore.getState().ensureAiMessage(currentId, tempKey);
      useChatStore.getState().replaceAiAnswer(currentId, tempKey, SENSITIVE_OUTPUT_BLOCKED_TOKEN, {
        sensitiveOutputBlocked: true,
      });
    };

    // Prepare callbacks
    const callbacks = {
      onStarted: (ctx: {
        conversationId: string;
        messageId?: string;
        workflowRunId?: string;
        tempKey?: string;
      }) => {
        this.store.getState().setIsSending(true);
        this.store.getState().setIsPaused(false);
        // Adopt server conversation id if this was a draft
        if (
          (!conv.conversationId || conv.conversationId.trim().length === 0) &&
          ctx.conversationId &&
          ctx.conversationId.trim().length > 0
        ) {
          this.adoptServerConversationId(currentId, ctx.conversationId);
          currentId = ctx.conversationId;
        }

        // Ensure AI message exists
        useChatStore.getState().ensureAiMessage(currentId, tempKey);
        useChatStore.getState().mergeAiMessage(currentId, tempKey, {
          messageId: ctx.messageId,
          workflowRunId: ctx.workflowRunId,
          conversationId: ctx.conversationId,
        });
        useChatStore.getState().resumeAiMessage(currentId, tempKey, {
          workflowRunId: ctx.workflowRunId,
        });
      },
      onToken: (token: string) => {
        textThrottler.append(token);
      },
      onTextReplace: () => {
        replaceWithBlockedAnswer();
      },
      onMessage: (meta?: Record<string, unknown>) => {
        const m = (meta ?? {}) as Record<string, unknown>;
        let chunk = '';
        if (typeof m['answer'] === 'string') chunk = m['answer'] as string;
        else if (typeof m['text'] === 'string') chunk = m['text'] as string;
        else if (typeof m['content'] === 'string') chunk = m['content'] as string;
        else if (typeof m['delta'] === 'string') chunk = m['delta'] as string;
        else if (m['outputs'] && typeof m['outputs'] === 'object') {
          const out = m['outputs'] as Record<string, unknown>;
          if (typeof out['answer'] === 'string') chunk = out['answer'] as string;
          else if (typeof out['text'] === 'string') chunk = out['text'] as string;
        }
        const serverConvId =
          typeof m['conversation_id'] === 'string' ? (m['conversation_id'] as string) : '';
        const messageId = typeof m['message_id'] === 'string' ? (m['message_id'] as string) : '';
        const workflowRunId =
          (typeof m['workflow_run_id'] === 'string' ? (m['workflow_run_id'] as string) : '') ||
          (typeof m['id'] === 'string' ? (m['id'] as string) : '');
        if ((!conv.conversationId || conv.conversationId.trim().length === 0) && serverConvId) {
          // Check if we haven't migrated yet (currentId is still activeId aka draft)
          if (currentId === activeId) {
            this.adoptServerConversationId(currentId, serverConvId);
            currentId = serverConvId;
          }
        }
        if (chunk.length > 0) {
          const currentMessage = useChatStore
            .getState()
            .conversations[currentId]?.messages.find(item => item.messageData?.tempKey === tempKey);
        const shouldReplacePersistentAnswer =
          Boolean(messageId || workflowRunId) &&
          (currentMessage?.WorkflowRunInfo?.status === 'pending_approval' ||
              currentMessage?.WorkflowRunInfo?.status === 'pending_question' ||
              currentMessage?.clientState?.status === 'pending_approval' ||
              currentMessage?.clientState?.status === 'pending_question');

          if (shouldReplacePersistentAnswer) {
            const answerMode = resolveAnswerMergeMode(currentMessage?.answer ?? '', chunk);
            if (answerMode === 'replace') {
              textThrottler.cancel();
            } else if (answerMode === 'append') {
              textThrottler.append(chunk);
            }
            useChatStore.getState().mergeAiMessage(currentId, tempKey, {
              ...(answerMode !== 'skip' && answerMode === 'replace'
                ? { answer: chunk, answerMode }
                : {}),
              messageId: messageId || undefined,
              workflowRunId: workflowRunId || undefined,
              conversationId: serverConvId || undefined,
            });
          } else {
            textThrottler.append(chunk);
            if (messageId || workflowRunId || serverConvId) {
              useChatStore.getState().mergeAiMessage(currentId, tempKey, {
                messageId: messageId || undefined,
                workflowRunId: workflowRunId || undefined,
                conversationId: serverConvId || undefined,
              });
            }
          }
        }

        // Handle generated images streaming (e.g. loading skeletons)
        if (m['generatedImages']) {
          useChatStore.getState().ensureAiMessage(currentId, tempKey);
          useChatStore
            .getState()
            .updateGeneratedImages(currentId, tempKey, m['generatedImages'] as GeneratedImage[]);
        }
      },
      onMessageEnd: (meta?: Record<string, unknown>) => {
        textThrottler.flush();
        if (meta) {
          const messageId =
            typeof meta.message_id === 'string' ? (meta.message_id as string) : undefined;
          const workflowRunId =
            (typeof meta.workflow_run_id === 'string' ? (meta.workflow_run_id as string) : '') ||
            (typeof meta.id === 'string' ? (meta.id as string) : '');
          const conversationId =
            typeof meta.conversation_id === 'string' ? (meta.conversation_id as string) : undefined;
          const metadata =
            meta.metadata && typeof meta.metadata === 'object'
              ? (meta.metadata as Record<string, unknown>)
              : undefined;
          useChatStore.getState().mergeAiMessage(currentId, tempKey, {
            messageId,
            workflowRunId: workflowRunId || undefined,
            conversationId,
            metadata,
          });
        }
      },
      mergeMessageData: (data: Record<string, unknown>) => {
        useChatStore.getState().mergeAiMessage(currentId, tempKey, { messageData: data });
      },
      onNodeStarted: (node: NodeInfo) => {
        const message = useChatStore
          .getState()
          .conversations[currentId]?.messages.find(item => item.messageData?.tempKey === tempKey);
        if (message?.clientState?.status === 'stopped') return;
        this.store.getState().setIsSending(true);
        this.store.getState().setIsPaused(false);
        useChatStore.getState().resumeAiMessage(currentId, tempKey);
        useChatStore.getState().updateRunNode(currentId, tempKey, node);
      },
      onNodeFinished: (node: NodeInfo) => {
        useChatStore.getState().updateRunNode(currentId, tempKey, node);
      },
      onPaused: (meta?: {
        elapsedTime?: number;
        workflowRunId?: string;
        nodeIds?: string[];
        status?: 'pending_approval' | 'pending_question';
        nodeType?: string;
      }) => {
        textThrottler.flush();
        useChatStore.getState().pauseAiMessage(currentId, tempKey, meta);
        meta?.nodeIds?.forEach(nodeId => {
          useChatStore.getState().updateRunNode(currentId, tempKey, {
            status: 'paused',
            nodeId,
            nodeType: meta.nodeType ?? 'approval',
          });
        });
        this.store.getState().setIsPaused(true);
        this.store.getState().setIsSending(false);
      },
      onFinished: (meta: {
        status: TerminalRunStatus;
        error?: string;
        elapsedTime?: number;
        messageId?: string;
        workflowRunId?: string;
        model?: ModelInfo | null;
      }) => {
        textThrottler.flush();
        textThrottler.cancel();

        useChatStore.getState().finalizeAiMessage(currentId, tempKey, meta);
        this.store.getState().setIsPaused(false);
        this.store.getState().setIsSending(false);

        // Refresh list to update dialogue count and timestamp
        const now = Date.now();
        const conversations = this.store.getState().conversations;
        const nextConversations = conversations
          .map(c =>
            c.id === currentId
              ? {
                  ...c,
                  dialogueCount:
                    meta.status === 'completed'
                      ? (c.dialogueCount ?? 0) + 1
                      : (c.dialogueCount ?? 0),
                  updatedAt: now,
                }
              : c
          )
          .sort((a, b) => (b.updatedAt ?? 0) - (a.updatedAt ?? 0));
        this.store.getState().setConversations(nextConversations);

        const currentDetail = this.store.getState().activeDetail;
        if (currentDetail?.summary.id === currentId) {
          const inc = meta.status === 'completed' ? 1 : 0;
          this.store.getState().setActiveDetail({
            ...currentDetail,
            summary: {
              ...currentDetail.summary,
              dialogueCount: (currentDetail.summary.dialogueCount ?? 0) + inc,
              updatedAt: now,
            },
          });
        }

        if (meta.status === 'completed') {
          this.scheduleConversationTitleRefresh(currentId);
        }
      },
      onError: (err: Error) => {
        console.error('[SingleChatController] Send error:', err);
        useChatStore.getState().finalizeAiMessage(currentId, tempKey, {
          status: 'error',
          error: err.message,
        });
        this.store.getState().setIsPaused(false);
        this.store.getState().setIsSending(false);
      },
    };

    // Call transport send
    this.transport.send(
      {
        query: payload.query,
        conversationId: requestConversationId,
        historyWindowSize: payload.historyWindowSize,
        files: payload.files,
        inputs: payload.inputs,
      },
      callbacks
    );
  }

  adoptServerConversationId(clientId: string, serverConversationId: string): void {
    this.store.getState().adoptServerConversationId(clientId, serverConversationId);
  }

  async rename(id: string, title: string): Promise<void> {
    if (!this.transport.rename) {
      console.warn('[SingleChatController] Rename not supported by transport');
      return;
    }

    const conv = this.store.getState().conversations.find(c => c.id === id);
    if (!conv) return;

    try {
      await this.transport.rename(conv.conversationId, title);

      // Update local state
      const nextConversations = this.store
        .getState()
        .conversations.map(c => (c.id === id ? { ...c, title } : c));
      this.store.getState().setConversations(nextConversations);

      const activeDetail = this.store.getState().activeDetail;
      if (activeDetail?.summary.id === id) {
        this.store.getState().setActiveDetail({
          ...activeDetail,
          summary: { ...activeDetail.summary, title },
        });
      }
    } catch (err) {
      console.error('[SingleChatController] Failed to rename:', err);
    }
  }

  async search(query: string, limit: number) {
    if (!this.transport.search) {
      const normalizedQuery = query.trim().toLowerCase();
      if (!normalizedQuery) return [];
      return this.store
        .getState()
        .conversations.filter(conversation => conversation.title.toLowerCase().includes(normalizedQuery))
        .slice(0, limit)
        .map<ConversationSearchResult>(conversation => ({
          type: 'conversation',
          conversationId: conversation.id,
          conversationTitle: conversation.title,
          snippet: conversation.title,
          updatedAt: conversation.updatedAt,
        }));
    }
    return this.transport.search(query, limit);
  }

  subscribe(
    listener: (state: SingleChatControllerState, prevState: SingleChatControllerState) => void
  ): () => void {
    return this.store.subscribe(listener);
  }
}

export default SingleChatController;
