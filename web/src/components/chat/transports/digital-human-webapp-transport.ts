import { AgentRuntimeTransport } from '@/components/chat/transports/agent-runtime-transport';
import { createDigitalHumanWebAppHttp } from '@/lib/http';
import type { AIChatChatRequest } from '@/services/types/aichat';

function toDigitalHumanWebAppChatRequest(payload: AIChatChatRequest): Record<string, unknown> {
  return {
    request_id: crypto.randomUUID(),
    query: payload.query,
    conversation_id: payload.conversation_id,
    parent_message_id: payload.parent_id,
    file_ids: payload.file_ids ?? [],
    response_mode: 'streaming',
  };
}

export function createDigitalHumanWebAppTransport(webAppId: string): AgentRuntimeTransport {
  const basePath = `/api/v1/digital-human-webapps/${encodeURIComponent(webAppId)}`;
  return new AgentRuntimeTransport({
    runtimeBasePath: basePath,
    chatPath: `${basePath}/chat`,
    client: createDigitalHumanWebAppHttp(webAppId),
    supportsRuntimeInput: true,
    transformChatPayload: toDigitalHumanWebAppChatRequest,
  });
}
