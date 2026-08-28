import * as React from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type {
  ChatRunCallbacks,
  ConversationDetail,
  ConversationSearchResult,
  ConversationSummary,
  ConversationTransport,
  Pagination,
  SendMessagePayload,
} from '@/components/chat/controllers/types';
import type { ClientMessageState, Conversation, GeneratedImage, Message } from '@/components/chat/types';
import { useChatStore } from '@/components/chat/store';
import { aichatService } from '@/services/aichat.service';
import { ImageRuntimeService } from '@/services/image-runtime.service';
import type { AIChatConversation, AIChatMessage, AIChatSearchResult } from '@/services/types/aichat';
import type {
  ImageRuntimeGeneration,
  ImageRuntimeModel,
  ImageRuntimeTask,
} from '@/services/types/image-runtime';
import { generateClientId } from '@/utils/client-id';
import { useT } from '@/i18n/translations';
import { IMAGE_RUNTIME_KEYS } from './use-image-runtime-models';

interface UseImageRuntimeTransportOptions {
  models: ImageRuntimeModel[];
}

const IMAGE_TASK_POLL_INTERVAL_MS = 5000;
const IMAGE_TASK_CLIENT_POLL_MAX_MS = 10 * 60 * 1000;
const IMAGE_TASK_TIMEOUT_ERROR = 'IMAGE_TASK_TIMEOUT';
const IMAGE_PROMPT_MAX_CHARACTERS = 4000;

interface ImageRuntimeText {
  generated: string;
  cancelled: string;
  timeout: string;
  providerFailed: string;
  failed: string;
  stillRunning: string;
  promptTooLong: string;
}

export function useImageRuntimeTransport(_options: UseImageRuntimeTransportOptions) {
  const t = useT('webapp');
  const queryClient = useQueryClient();
  const recoveringTaskIdsRef = React.useRef(new Set<string>());
  const imageRuntimeText: ImageRuntimeText = {
    generated: t('chat.imageInput.taskGenerated'),
    cancelled: t('chat.imageInput.taskCancelled'),
    timeout: t('chat.imageInput.taskTimeout'),
    providerFailed: t('chat.imageInput.taskProviderFailed'),
    failed: t('chat.imageInput.taskFailed'),
    stillRunning: t('chat.imageInput.taskStillRunning'),
    promptTooLong: t('chat.imageInput.promptTooLong', { max: IMAGE_PROMPT_MAX_CHARACTERS }),
  };
  const imageRuntimeTextRef = React.useRef(imageRuntimeText);
  imageRuntimeTextRef.current = imageRuntimeText;

  React.useEffect(() => {
    let disposed = false;
    const pollRecoveredTasks = async () => {
      try {
        const resp = await ImageRuntimeService.listTasks({ limit: 20 });
        const tasks = resp.data?.data ?? [];
        tasks.filter(isExpiredImageTask).forEach(task => {
          applyRecoveredTaskToChatStore(taskWithTimeoutDisplay(task), imageRuntimeTextRef.current);
        });
        const activeTasks = tasks.filter(isActiveImageTask);
        if (activeTasks.length === 0) return;
        await Promise.all(
          activeTasks
            .filter(task => {
              if (recoveringTaskIdsRef.current.has(task.task_id)) return false;
              recoveringTaskIdsRef.current.add(task.task_id);
              return true;
            })
            .map(task =>
              waitForImageTask(task, {
                onTerminal: terminalTask => {
                  applyRecoveredTaskToChatStore(terminalTask, imageRuntimeTextRef.current);
                  void queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
                  void queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.taskLists });
                },
                shouldStop: () => disposed,
              }).finally(() => {
                recoveringTaskIdsRef.current.delete(task.task_id);
              })
            )
        );
      } catch (_error) {
        // Recovery is opportunistic; visible send flows handle user-facing errors.
      }
    };

    void pollRecoveredTasks();
    const handleResume = () => {
      if (!document.hidden) {
        void pollRecoveredTasks();
      }
    };
    window.addEventListener('focus', handleResume);
    document.addEventListener('visibilitychange', handleResume);
    return () => {
      disposed = true;
      window.removeEventListener('focus', handleResume);
      document.removeEventListener('visibilitychange', handleResume);
    };
  }, [queryClient]);

  const transport = React.useMemo<ConversationTransport>(
    () => ({
      async list(params: {
        page: number;
        limit: number;
      }): Promise<{ items: ConversationSummary[]; pagination: Pagination }> {
        const resp = await queryClient.fetchQuery({
          queryKey: [...IMAGE_RUNTIME_KEYS.conversations, params],
          queryFn: () =>
            aichatService.listConversations({ ...params, conversation_type: 'image' }),
          staleTime: 30 * 1000,
          retry: false,
        });
        const { data, page, limit, total, has_more } = resp.data;
        return {
          items: data.map(mapConversationToSummary),
          pagination: { page, limit, total, hasMore: has_more },
        };
      },

      async get(conversationId: string): Promise<ConversationDetail> {
        const [conversationResp, messagesResp] = await Promise.all([
          aichatService.getConversation(conversationId, 'image'),
          aichatService.listMessages(conversationId, {
            page: 1,
            limit: 200,
            conversation_type: 'image',
          }),
        ]);
        const summary = mapConversationToSummary(conversationResp.data);
        return {
          summary,
          messages: [...messagesResp.data.data]
            .sort((a, b) => a.created_at - b.created_at)
            .map(item => mapMessage(item, imageRuntimeTextRef.current)),
          loaded: true,
          loading: false,
        };
      },

      async search(query: string, limit: number): Promise<ConversationSearchResult[]> {
        const normalized = query.trim();
        if (!normalized) return [];
        const resp = await aichatService.search(normalized, limit, { conversation_type: 'image' });
        return resp.data.map(mapSearchResult);
      },

      async create(payload?: { title?: string }): Promise<ConversationSummary> {
        return {
          id: `draft-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
          conversationId: '',
          title: payload?.title ?? 'New conversation',
          dialogueCount: 0,
          updatedAt: Date.now(),
          status: 'draft',
        };
      },

      async remove(conversationId: string): Promise<void> {
        if (!conversationId || conversationId.startsWith('draft-')) return;
        await aichatService.deleteConversation(conversationId, 'image');
        await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
      },

      send(payload: SendMessagePayload, callbacks: ChatRunCallbacks, abortSignal?: AbortSignal): void {
        void sendImageRuntimeMessage(
          payload,
          callbacks,
          abortSignal,
          queryClient,
          imageRuntimeTextRef.current
        );
      },
    }),
    [queryClient]
  );

  return { transport };
}

async function sendImageRuntimeMessage(
  payload: SendMessagePayload,
  callbacks: ChatRunCallbacks,
  abortSignal: AbortSignal | undefined,
  queryClient: ReturnType<typeof useQueryClient>,
  text: ImageRuntimeText
) {
  const modelConfig = objectValue(payload.inputs?.model_config);
  const provider = stringValue(modelConfig?.provider);
  const model = stringValue(modelConfig?.model);
  const imageOptions = objectValue(payload.inputs?.image_gen_config);
  const imageReference = objectValue(payload.inputs?.image_reference);
  const referenceImageFileId = stringValue(imageReference?.file_id);
  const clientRequestId = generateClientId('image-request');
  let submittedTask: ImageRuntimeTask | null = null;

  try {
    const resp = await ImageRuntimeService.generate(
      {
        prompt: payload.query,
        provider,
        model,
        client_request_id: clientRequestId,
        options: {
          size: optionalStringValue(imageOptions?.size),
          count: numberValue(imageOptions?.count),
          generation_mode: generationModeValue(imageOptions?.generation_mode),
          max_images: numberValue(imageOptions?.max_images),
        },
        conversation_id: payload.conversationId || undefined,
        ...(referenceImageFileId
          ? {
              reference_image: {
                file_id: referenceImageFileId,
                url: optionalStringValue(imageReference?.url),
                filename: optionalStringValue(imageReference?.filename),
                mime_type: optionalStringValue(imageReference?.mime_type),
              },
            }
          : {}),
      },
      abortSignal
    );
    submittedTask = resp.data.task;
    await handleSubmittedImageTask(submittedTask, callbacks, queryClient, text);
  } catch (error) {
    const recoveredTask = await findImageTaskByClientRequestId(clientRequestId).catch(() => null);
    if (recoveredTask) {
      submittedTask = recoveredTask;
      await handleSubmittedImageTask(recoveredTask, callbacks, queryClient, text);
      return;
    }
    if (submittedTask && isActiveImageTask(submittedTask)) {
      callbacks.onToken(text.stillRunning);
      callbacks.onFinished({ status: 'completed' });
      await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
      return;
    }
    const err = error instanceof Error ? error : new Error('Image generation failed');
    callbacks.onError(err);
    callbacks.onFinished({ status: 'error', error: err.message });
  }
}

async function handleSubmittedImageTask(
  initialTask: ImageRuntimeTask,
  callbacks: ChatRunCallbacks,
  queryClient: ReturnType<typeof useQueryClient>,
  text: ImageRuntimeText
) {
  let task = initialTask;
  if (task.conversation_id) {
    callbacks.onStarted({ conversationId: task.conversation_id, messageId: task.message_id });
  }
  callbacks.mergeMessageData?.({
    conversation_id: task.conversation_id,
    message_id: task.message_id,
    image_task: task,
    image_task_id: task.task_id,
    image_task_status: task.status,
    image_generation: task.image_generation ?? imageGenerationFromTask(task),
    model_label: task.model_label,
    model_name: task.model,
    model_provider: task.provider,
    created_at: Math.floor(Date.now() / 1000),
  });
  callbacks.onMessage({
    conversation_id: task.conversation_id,
    message_id: task.message_id,
    generatedImages: displayGeneratedImagesForTask(task),
  });

  task = await waitForImageTask(task, {
    onUpdate: nextTask => {
      callbacks.mergeMessageData?.({
        conversation_id: nextTask.conversation_id,
        message_id: nextTask.message_id,
        image_task: nextTask,
        image_task_id: nextTask.task_id,
        image_task_status: nextTask.status,
        image_generation: nextTask.image_generation ?? imageGenerationFromTask(nextTask),
      });
      if (!isActiveImageTask(nextTask)) {
        callbacks.onMessage({
          conversation_id: nextTask.conversation_id,
          message_id: nextTask.message_id,
          generatedImages: displayGeneratedImagesForTask(nextTask),
        });
      }
    },
  });

  if (task.status === 'succeeded' && task.image_generation) {
    const generatedImages = generatedImagesFromGeneration(task.image_generation);
    if (task.conversation_id) {
      callbacks.onStarted({ conversationId: task.conversation_id, messageId: task.message_id });
    }
    callbacks.mergeMessageData?.({
      conversation_id: task.conversation_id,
      message_id: task.message_id,
      metadata: { image_generation: task.image_generation },
      image_generation: task.image_generation,
      image_task: task,
      image_task_id: task.task_id,
      image_task_status: task.status,
      model_label: task.image_generation.model_label,
      model_name: task.image_generation.model,
      model_provider: task.image_generation.provider,
      created_at: Math.floor(Date.now() / 1000),
      generatedImages,
    });
    callbacks.onMessage({ generatedImages });
    callbacks.onToken(text.generated);
    callbacks.onFinished({
      status: 'completed',
      messageId: task.message_id,
      generatedImages,
      model: { modelName: task.image_generation.model, provider: task.image_generation.provider },
    });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.taskLists });
    return;
  }

  if (task.status === 'failed') {
    const message = imageTaskErrorDisplayText(task.error_message, text);
    callbacks.mergeMessageData?.({
      conversation_id: task.conversation_id,
      message_id: task.message_id,
      image_task: task,
      image_task_id: task.task_id,
      image_task_status: task.status,
      image_generation: task.image_generation ?? imageGenerationFromTask(task),
      image_task_error: task.error_message,
    });
    callbacks.onMessage({
      conversation_id: task.conversation_id,
      message_id: task.message_id,
      generatedImages: [],
    });
    callbacks.onToken(message);
    callbacks.onFinished({ status: 'error', error: message, messageId: task.message_id });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.taskLists });
    return;
  }

  if (task.status === 'cancelled') {
    callbacks.mergeMessageData?.({
      conversation_id: task.conversation_id,
      message_id: task.message_id,
      image_task: task,
      image_task_id: task.task_id,
      image_task_status: task.status,
      image_generation: task.image_generation ?? imageGenerationFromTask(task),
    });
    callbacks.onMessage({
      conversation_id: task.conversation_id,
      message_id: task.message_id,
      generatedImages: [],
    });
    callbacks.onToken(text.cancelled);
    callbacks.onFinished({ status: 'stopped', messageId: task.message_id });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.taskLists });
    return;
  }

  callbacks.onToken(text.stillRunning);
  callbacks.onFinished({ status: 'completed' });
}

async function waitForImageTask(
  initialTask: ImageRuntimeTask,
  options: {
    onUpdate?: (task: ImageRuntimeTask) => void;
    onTerminal?: (task: ImageRuntimeTask) => void;
    shouldStop?: () => boolean;
  } = {}
): Promise<ImageRuntimeTask> {
  let task = initialTask;
  const startedAt = Date.now();
  let pollFailures = 0;
  while (isActiveImageTask(task) && Date.now() - startedAt < IMAGE_TASK_CLIENT_POLL_MAX_MS) {
    if (options.shouldStop?.()) break;
    await sleep(IMAGE_TASK_POLL_INTERVAL_MS);
    if (options.shouldStop?.()) break;
    try {
      const resp = await ImageRuntimeService.getTask(task.task_id);
      task = resp.data;
      pollFailures = 0;
      options.onUpdate?.(task);
    } catch (_error) {
      pollFailures += 1;
      if (pollFailures >= 5) {
        break;
      }
    }
  }
  if (isActiveImageTask(task) && Date.now() - startedAt >= IMAGE_TASK_CLIENT_POLL_MAX_MS) {
    task = taskWithTimeoutDisplay(task);
    options.onUpdate?.(task);
  }
  if (!isActiveImageTask(task)) {
    options.onTerminal?.(task);
  }
  return task;
}

async function findImageTaskByClientRequestId(clientRequestId: string): Promise<ImageRuntimeTask | null> {
  const resp = await ImageRuntimeService.listTasks({ limit: 10, search: clientRequestId });
  return (resp.data?.data ?? []).find(task => task.client_request_id === clientRequestId) ?? null;
}

function isActiveImageTask(task: Pick<ImageRuntimeTask, 'status'>) {
  const status = task.status?.toLowerCase?.() ?? '';
  if (isExpiredImageTask(task)) {
    return false;
  }
  return status === 'pending' || status === 'running' || status === 'processing' || status === 'in_progress';
}

function isExpiredImageTask(task: Pick<ImageRuntimeTask, 'status'> & Partial<Pick<ImageRuntimeTask, 'created_at'>>) {
  const status = task.status?.toLowerCase?.() ?? '';
  const isActive =
    status === 'pending' || status === 'running' || status === 'processing' || status === 'in_progress';
  return isActive && isOlderThanImageTaskTimeout(task.created_at);
}

function taskWithTimeoutDisplay(task: ImageRuntimeTask): ImageRuntimeTask {
  return {
    ...task,
    status: 'failed',
    error_message: task.error_message || IMAGE_TASK_TIMEOUT_ERROR,
    image_generation: task.image_generation
      ? { ...task.image_generation, status: 'failed', files: task.image_generation.files ?? [] }
      : undefined,
  };
}

function loadingGeneratedImages(task: ImageRuntimeTask): GeneratedImage[] {
  const count = Math.max(1, Math.min(task.count || 1, 4));
  return Array.from({ length: count }, (_, index) => ({
    url: '',
    alt: `Generating image ${index + 1}`,
    isLoading: true,
  }));
}

function displayGeneratedImagesForTask(task: ImageRuntimeTask): GeneratedImage[] {
  if (task.status === 'succeeded' && task.image_generation) {
    return generatedImagesFromGeneration(task.image_generation);
  }
  return isActiveImageTask(task) ? loadingGeneratedImages(task) : [];
}

function sleep(ms: number) {
  return new Promise(resolve => window.setTimeout(resolve, ms));
}

function mapConversationToSummary(item: AIChatConversation): ConversationSummary {
  return {
    id: item.id,
    conversationId: item.id,
    title: item.title,
    dialogueCount: item.dialogue_count,
    updatedAt: item.updated_at * 1000,
    status: item.status,
    metadata: item.metadata,
  };
}

function mapMessage(item: AIChatMessage, text: ImageRuntimeText): Message {
  const rawImageGeneration = objectValue(item.metadata?.image_generation) as ImageRuntimeGeneration | undefined;
  const rawImageTaskStatus = stringValue(item.metadata?.image_task_status) || rawImageGeneration?.status || item.status;
  const isExpiredImageMessage = isExpiredRuntimeImageMessage(item, rawImageTaskStatus);
  const imageTaskStatus = resolveImageTaskStatus(
    rawImageTaskStatus,
    item.status,
    item.error,
    isExpiredImageMessage
  );
  const imageGeneration = rawImageGeneration
    ? ({ ...rawImageGeneration, status: imageTaskStatus } as ImageRuntimeGeneration)
    : undefined;
  const modelLabel = imageGeneration?.model_label || item.model_name;
  const imageTaskTerminal = isTerminalImageStatus(imageTaskStatus);
  const isGenerating =
    !imageTaskTerminal &&
    (isActiveImageStatus(imageTaskStatus) || item.status === 'pending' || item.status === 'streaming');
  const imageErrorText = imageTaskErrorDisplayText(
    isExpiredImageMessage
      ? IMAGE_TASK_TIMEOUT_ERROR
      : stringValue(item.metadata?.image_task_error) || item.error || imageTaskStatus,
    text
  );
  const isImageError = item.status === 'error' || isFailedImageStatus(imageTaskStatus);
  const answer =
    isImageError && isInternalImageTaskError(item.answer)
      ? imageTaskErrorDisplayText(item.answer, text)
      : item.answer || (isImageError ? imageErrorText : '');
  return {
    messageId: item.id,
    query: item.query,
    answer,
    parentId: item.parent_id ?? '',
    model: item.model_name
      ? { modelName: modelLabel, provider: item.model_provider, rawModelName: item.model_name }
      : null,
    clientState: {
      phase: isGenerating ? 'streaming' : 'completed',
      status:
        isFailedImageStatus(imageTaskStatus)
          ? 'error'
          : imageTaskStatus === 'cancelled'
            ? 'stopped'
            : item.status === 'completed'
          ? 'completed'
          : item.status === 'error'
            ? 'error'
            : item.status === 'stopped'
              ? 'stopped'
              : undefined,
      finishedAt: item.updated_at * 1000,
    },
    messageData: {
      conversation_id: item.conversation_id,
      message_id: item.id,
      metadata: item.metadata,
      image_generation: imageGeneration,
      image_task_id: stringValue(item.metadata?.image_task_id),
      image_task_status: imageTaskStatus,
      model_label: modelLabel,
      model_name: item.model_name,
      model_provider: item.model_provider,
      created_at: item.created_at,
      status: item.status,
    },
    generatedImages: imageGeneration
      ? generatedImagesFromGeneration(imageGeneration)
      : isGenerating
        ? loadingGeneratedImages(imageTaskFromMessage(item))
        : undefined,
  };
}

function mapSearchResult(item: AIChatSearchResult): ConversationSearchResult {
  return {
    type: item.type,
    conversationId: item.conversation_id,
    conversationTitle: item.conversation_title,
    messageId: item.message_id,
    snippet: item.snippet,
    updatedAt: item.updated_at * 1000,
  };
}

function generatedImagesFromGeneration(generation: ImageRuntimeGeneration): GeneratedImage[] {
  if (isActiveImageStatus(generation.status) && (generation.files?.length ?? 0) === 0) {
    const count = Math.max(1, Math.min(generation.count || 1, 4));
    return Array.from({ length: count }, (_, index) => ({
      url: '',
      alt: `Generating image ${index + 1}`,
      isLoading: true,
    }));
  }
  return (generation.files ?? []).map(file => ({
    url: file.url || file.download_url,
    alt: file.filename,
  }));
}

function imageGenerationFromTask(task: ImageRuntimeTask): ImageRuntimeGeneration {
  return {
    provider: task.provider,
    model: task.model,
    model_label: task.model_label || task.model,
    size: task.size || '',
    count: task.count || 1,
    generation_mode: task.generation_mode,
    max_images: task.max_images,
    files: task.files ?? [],
    reference_image: task.reference_image,
    status: task.status,
  };
}

function imageTaskFromMessage(item: AIChatMessage): ImageRuntimeTask {
  const generation = objectValue(item.metadata?.image_generation) as ImageRuntimeGeneration | undefined;
  return {
    id: stringValue(item.metadata?.image_task_id) || item.id,
    task_id: stringValue(item.metadata?.image_task_id) || item.id,
    client_request_id: stringValue(item.metadata?.client_request_id),
    conversation_id: item.conversation_id,
    message_id: item.id,
    provider: generation?.provider || item.model_provider || '',
    model: generation?.model || item.model_name,
    model_label: generation?.model_label || item.model_name,
    prompt: item.query,
    status: stringValue(item.metadata?.image_task_status) || generation?.status || item.status,
    size: generation?.size,
    count: generation?.count || 1,
    generation_mode: generation?.generation_mode,
    max_images: generation?.max_images,
    files: generation?.files ?? [],
    reference_image: generation?.reference_image,
    created_at: new Date(item.created_at * 1000).toISOString(),
    updated_at: new Date(item.updated_at * 1000).toISOString(),
  };
}

function isExpiredRuntimeImageMessage(item: AIChatMessage, rawStatus: unknown): boolean {
  const hasImageRuntimeMetadata = Boolean(
    stringValue(item.metadata?.image_runtime_kind) === 'generation' ||
      stringValue(item.metadata?.image_task_id) ||
      objectValue(item.metadata?.image_generation)
  );
  return hasImageRuntimeMetadata && isActiveImageStatus(rawStatus) && isOlderThanImageTaskTimeout(item.created_at);
}

function applyRecoveredTaskToChatStore(task: ImageRuntimeTask, text: ImageRuntimeText) {
  useChatStore.setState(state => {
    let changed = false;
    const nextConversations: Record<string, Conversation> = {};
    for (const [id, conversation] of Object.entries(state.conversations)) {
      let conversationChanged = false;
      const nextMessages = conversation.messages.map(message => {
        const matches =
          message.messageId === task.message_id ||
          message.messageData?.message_id === task.message_id ||
          message.messageData?.image_task_id === task.task_id;
        if (!matches) return message;
        changed = true;
        const imageGeneration = task.image_generation ?? imageGenerationFromTask(task);
        const generatedImages =
          task.status === 'succeeded' && task.image_generation
            ? generatedImagesFromGeneration(task.image_generation)
            : isActiveImageTask(task)
              ? loadingGeneratedImages(task)
              : [];
        const clientState: ClientMessageState = {
          ...message.clientState,
          phase: isActiveImageTask(task) ? 'streaming' : 'completed',
          status:
            task.status === 'succeeded'
              ? 'completed'
              : task.status === 'failed'
                ? 'error'
                : task.status === 'cancelled'
                  ? 'stopped'
                  : message.clientState?.status,
        };
        conversationChanged = true;
        return {
          ...message,
          answer:
            task.status === 'succeeded'
              ? text.generated
              : task.status === 'failed'
                ? imageTaskErrorDisplayText(task.error_message, text)
                : task.status === 'cancelled'
                  ? text.cancelled
                  : message.answer,
          clientState,
          generatedImages,
          messageData: {
            ...message.messageData,
            conversation_id: task.conversation_id,
            message_id: task.message_id,
            image_generation: imageGeneration,
            image_task: task,
            image_task_id: task.task_id,
            image_task_status: task.status,
            model_label: task.model_label,
            model_name: task.model,
            model_provider: task.provider,
          },
        };
      });
      nextConversations[id] = conversationChanged
        ? { ...conversation, messages: nextMessages }
        : conversation;
    }
    if (!changed) return state;
    return { conversations: nextConversations };
  });
}

function isActiveImageStatus(status: unknown): boolean {
  const normalized = stringValue(status).toLowerCase();
  if (isTerminalImageStatus(normalized)) {
    return false;
  }
  return (
    normalized === 'pending' ||
    normalized === 'running' ||
    normalized === 'processing' ||
    normalized === 'in_progress' ||
    normalized === 'streaming'
  );
}

function isTerminalImageStatus(status: unknown): boolean {
  const normalized = stringValue(status).toLowerCase();
  return (
    normalized === 'succeeded' ||
    normalized === 'failed' ||
    normalized === 'cancelled' ||
    normalized === 'runtime_lease_expired' ||
    normalized === 'image_task_timeout' ||
    normalized === 'timeout'
  );
}

function isFailedImageStatus(status: unknown): boolean {
  const normalized = stringValue(status).toLowerCase();
  return (
    normalized === 'failed' ||
    normalized === 'runtime_lease_expired' ||
    normalized === 'image_task_timeout' ||
    normalized === 'timeout'
  );
}

function resolveImageTaskStatus(
  status: unknown,
  messageStatus: unknown,
  messageError: unknown,
  isExpiredImageMessage = false
): string {
  const normalized = stringValue(status).toLowerCase();
  const normalizedMessageStatus = stringValue(messageStatus).toLowerCase();
  const normalizedError = stringValue(messageError).toLowerCase();
  if (isExpiredImageMessage) {
    return 'failed';
  }
  if (
    normalizedMessageStatus === 'error' &&
    (isActiveImageStatus(normalized) ||
      normalized === 'runtime_lease_expired' ||
      normalizedError === 'runtime_lease_expired' ||
      normalizedError === 'image_task_timeout')
  ) {
    return 'failed';
  }
  if (normalized === 'runtime_lease_expired' || normalized === 'image_task_timeout' || normalized === 'timeout') {
    return 'failed';
  }
  return normalized;
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : undefined;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function optionalStringValue(value: unknown): string | undefined {
  const normalized = stringValue(value);
  return normalized || undefined;
}

function imageTaskErrorDisplayText(errorMessage: unknown, text: ImageRuntimeText): string {
  switch (stringValue(errorMessage).toLowerCase()) {
    case 'image_task_timeout':
    case 'runtime_lease_expired':
    case 'timeout':
      return text.timeout;
    case 'prompt_too_long':
      return text.promptTooLong;
    case 'upstream_failed':
      return text.providerFailed;
    default:
      return stringValue(errorMessage) || text.failed;
  }
}

function isInternalImageTaskError(errorMessage: unknown): boolean {
  const normalized = stringValue(errorMessage).toLowerCase();
  return (
    normalized === 'image_task_timeout' ||
    normalized === 'runtime_lease_expired' ||
    normalized === 'timeout' ||
    normalized === 'upstream_failed'
  );
}

function isOlderThanImageTaskTimeout(value: unknown): boolean {
  const timestamp = timestampMs(value);
  return timestamp > 0 && Date.now() - timestamp >= IMAGE_TASK_CLIENT_POLL_MAX_MS;
}

function timestampMs(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value < 1_000_000_000_000 ? value * 1000 : value;
  }
  if (typeof value !== 'string') {
    return 0;
  }
  const trimmed = value.trim();
  if (!trimmed) {
    return 0;
  }
  const numeric = Number(trimmed);
  if (Number.isFinite(numeric)) {
    return numeric < 1_000_000_000_000 ? numeric * 1000 : numeric;
  }
  const parsed = Date.parse(trimmed);
  return Number.isFinite(parsed) ? parsed : 0;
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) ? value : undefined;
}

function generationModeValue(value: unknown): 'single' | 'sequence' | undefined {
  return value === 'single' || value === 'sequence' ? value : undefined;
}
