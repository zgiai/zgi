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
import type { GeneratedImage, Message } from '@/components/chat/types';
import { aichatService } from '@/services/aichat.service';
import { ImageRuntimeService } from '@/services/image-runtime.service';
import type { AIChatConversation, AIChatMessage, AIChatSearchResult } from '@/services/types/aichat';
import type { ImageRuntimeGeneration, ImageRuntimeModel } from '@/services/types/image-runtime';
import { IMAGE_RUNTIME_KEYS } from './use-image-runtime-models';

interface UseImageRuntimeTransportOptions {
  models: ImageRuntimeModel[];
}

export function useImageRuntimeTransport({ models }: UseImageRuntimeTransportOptions) {
  const queryClient = useQueryClient();
  const modelsRef = React.useRef(models);
  modelsRef.current = models;

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
            .map(mapMessage),
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
        void sendImageRuntimeMessage(payload, callbacks, modelsRef.current, abortSignal, queryClient);
      },
    }),
    [queryClient]
  );

  return { transport };
}

async function sendImageRuntimeMessage(
  payload: SendMessagePayload,
  callbacks: ChatRunCallbacks,
  models: ImageRuntimeModel[],
  abortSignal: AbortSignal | undefined,
  queryClient: ReturnType<typeof useQueryClient>
) {
  const modelConfig = objectValue(payload.inputs?.model_config);
  const provider = stringValue(modelConfig?.provider);
  const model = stringValue(modelConfig?.model);
  const runtimeModel = models.find(item => item.provider === provider && item.model === model);
  const size = resolveSize(runtimeModel, stringValue(objectValue(payload.inputs?.image_gen_config)?.aspect_ratio));
  const count = numberValue(objectValue(payload.inputs?.image_gen_config)?.n) ?? runtimeModel?.default_count ?? 1;

  try {
    const resp = await ImageRuntimeService.generate(
      {
        prompt: payload.query,
        provider,
        model,
        size,
        count,
        conversation_id: payload.conversationId || undefined,
      },
      abortSignal
    );
    const data = resp.data;
    const generatedImages = generatedImagesFromGeneration(data.image_generation);
    callbacks.onStarted({ conversationId: data.conversation_id, messageId: data.message_id });
    callbacks.mergeMessageData?.({
      conversation_id: data.conversation_id,
      message_id: data.message_id,
      metadata: { image_generation: data.image_generation },
      image_generation: data.image_generation,
      model_label: data.image_generation.model_label,
      model_name: data.image_generation.model,
      model_provider: data.image_generation.provider,
      created_at: Math.floor(Date.now() / 1000),
      generatedImages,
    });
    callbacks.onMessage({ generatedImages });
    callbacks.onToken(data.message);
    callbacks.onFinished({
      status: 'completed',
      messageId: data.message_id,
      generatedImages,
      model: { modelName: data.image_generation.model, provider: data.image_generation.provider },
    });
    await queryClient.invalidateQueries({ queryKey: IMAGE_RUNTIME_KEYS.conversations });
  } catch (error) {
    const err = error instanceof Error ? error : new Error('Image generation failed');
    callbacks.onError(err);
    callbacks.onFinished({ status: 'error', error: err.message });
  }
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

function mapMessage(item: AIChatMessage): Message {
  const imageGeneration = objectValue(item.metadata?.image_generation) as ImageRuntimeGeneration | undefined;
  const modelLabel = imageGeneration?.model_label || item.model_name;
  return {
    messageId: item.id,
    query: item.query,
    answer: item.answer,
    parentId: item.parent_id ?? '',
    model: item.model_name
      ? { modelName: modelLabel, provider: item.model_provider, rawModelName: item.model_name }
      : null,
    clientState: {
      phase: item.status === 'streaming' ? 'streaming' : 'completed',
      status: item.status === 'completed' ? 'completed' : item.status === 'error' ? 'error' : undefined,
      finishedAt: item.updated_at * 1000,
    },
    messageData: {
      conversation_id: item.conversation_id,
      message_id: item.id,
      metadata: item.metadata,
      image_generation: imageGeneration,
      model_label: modelLabel,
      model_name: item.model_name,
      model_provider: item.model_provider,
      created_at: item.created_at,
      status: item.status,
    },
    generatedImages: imageGeneration ? generatedImagesFromGeneration(imageGeneration) : undefined,
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
  return generation.files.map(file => ({
    url: file.url || file.download_url,
    alt: file.filename,
  }));
}

function resolveSize(model: ImageRuntimeModel | undefined, ratio: string): string {
  const fallback = model?.default_size ?? '1024x1024';
  const supportedSizes = model?.supported_sizes ?? [];
  const preferred = sizeForRatio(ratio, supportedSizes);
  if (preferred) return preferred;
  return fallback;
}

const RATIO_SIZE_CANDIDATES: Record<string, string[]> = {
  '1:1': ['1024x1024', '2048x2048'],
  '2:3': ['1024x1536'],
  '3:2': ['1536x1024'],
  '3:4': ['768x1024'],
  '4:3': ['1024x768'],
  '16:9': ['1792x1024', '2048x1152', '3840x2160'],
  '9:16': ['1024x1792', '2160x3840'],
};

function sizeForRatio(ratio: string, supportedSizes: string[]): string | undefined {
  const candidates = RATIO_SIZE_CANDIDATES[ratio] ?? RATIO_SIZE_CANDIDATES['1:1'];
  if (supportedSizes.length === 0) return candidates[0];
  return candidates.find(size => supportedSizes.includes(size));
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : undefined;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function numberValue(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isInteger(value) ? value : undefined;
}
