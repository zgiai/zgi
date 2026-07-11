import type {
  AIChatFileParseEndEventData,
  AIChatFileParseErrorEventData,
  AIChatFileParseStartEventData,
  AIChatGeneratedFile,
  AIChatMessageFile,
  AIChatSkillArtifactCreatedEventData,
} from '@/services/types/aichat';
import { type AIChatControllerState } from '@/components/chat/controllers/aichat/types';
import { isStaleAIChatStreamEvent } from './shared';

function inferExtension(filename: string): string {
  return filename.split('.').pop()?.toLowerCase() ?? '';
}

function upsertMessageFile(
  files: AIChatMessageFile[],
  fileId: string,
  fallbackName: string,
  updater: (file: AIChatMessageFile) => AIChatMessageFile
): AIChatMessageFile[] {
  const index = files.findIndex(file => file.id === fileId);
  const baseFile: AIChatMessageFile =
    index >= 0
      ? files[index]
      : {
          id: fileId,
          name: fallbackName,
          size: 0,
          extension: inferExtension(fallbackName),
          mime_type: '',
          workspace_id: null,
          is_temporary: true,
          content_status: 'pending',
          content_chars: 0,
          content_preview: '',
          from_cache: false,
          kind: 'document',
          vision_detail: null,
          filtered_reason: null,
          parse_status: 'pending',
        };
  const nextFile = updater(baseFile);
  if (index < 0) {
    return [...files, nextFile];
  }

  const nextFiles = files.slice();
  nextFiles[index] = nextFile;
  return nextFiles;
}

function updateMessageFileMetadata(
  current: AIChatControllerState,
  conversationId: string,
  messageId: string,
  fileId: string,
  fallbackName: string,
  eventId: string | null | undefined,
  updater: (file: AIChatMessageFile) => AIChatMessageFile
): AIChatControllerState {
  const previousStreaming = current.streamingByMessageId[messageId];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const messages = current.messagesByConversation[conversationId] ?? [];
  const nextMessages = messages.map(message => {
    if (message.id !== messageId) {
      return message;
    }

    const files = upsertMessageFile(message.metadata?.files ?? [], fileId, fallbackName, updater);

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        file_count: files.length,
        files,
      },
      updated_at: Math.floor(Date.now() / 1000),
    };
  });
  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [conversationId]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [messageId]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

function upsertGeneratedFile(
  files: AIChatGeneratedFile[],
  incoming: AIChatGeneratedFile
): AIChatGeneratedFile[] {
  const index = files.findIndex(
    file => generatedFileIdentity(file) === generatedFileIdentity(incoming)
  );
  if (index < 0) {
    return [...files, incoming];
  }

  const nextFiles = files.slice();
  nextFiles[index] = {
    ...nextFiles[index],
    ...incoming,
  };
  return nextFiles;
}

function generatedFileIdentity(file: AIChatGeneratedFile): string {
  if (file.artifact_id) return file.artifact_id;
  const isManaged = file.target === 'managed_file' || Boolean(file.upload_file_id);
  const fileId = file.upload_file_id || file.tool_file_id || file.file_id;
  return `${isManaged ? 'managed_file' : 'tool_file'}:${fileId}`;
}

function normalizeSkillArtifactFile(
  payload: AIChatSkillArtifactCreatedEventData
): AIChatGeneratedFile | null {
  const file = payload.file;
  const toolFileId = file?.tool_file_id ?? payload.tool_file_id;
  const uploadFileId = file?.upload_file_id ?? payload.upload_file_id;
  const sourceFileId = file?.source_file_id ?? payload.source_file_id;
  const sourceToolFileId = file?.source_tool_file_id ?? payload.source_tool_file_id;
  const fileId = file?.file_id ?? payload.file_id ?? uploadFileId ?? toolFileId;
  const filename = file?.filename ?? payload.filename;
  const extension = file?.extension ?? payload.extension;
  const mimeType = file?.mime_type ?? payload.mime_type;
  const size = file?.size ?? payload.size;
  const target = file?.target ?? payload.target;
  const url = file?.url ?? payload.url ?? '';
  const isManagedFile = target === 'managed_file' || Boolean(uploadFileId);

  if (
    !payload.skill_id ||
    !payload.tool_name ||
    !fileId ||
    !filename ||
    !extension ||
    !mimeType ||
    typeof size !== 'number' ||
    (!url && !isManagedFile)
  ) {
    return null;
  }

  return {
    artifact_id:
      file?.artifact_id ??
      payload.artifact_id ??
      `${isManagedFile ? 'managed_file' : 'tool_file'}:${fileId}`,
    artifact_type: 'file',
    skill_id: payload.skill_id,
    tool_name: payload.tool_name,
    file_id: fileId,
    tool_file_id: toolFileId,
    upload_file_id: uploadFileId,
    source_file_id: sourceFileId,
    source_tool_file_id: sourceToolFileId,
    filename,
    extension,
    mime_type: mimeType,
    size,
    url,
    download_url: file?.download_url ?? payload.download_url,
    target,
    lifecycle: file?.lifecycle ?? payload.lifecycle,
    expires_at: file?.expires_at ?? payload.expires_at,
    availability: file?.availability ?? payload.availability,
    transfer_method: file?.transfer_method ?? payload.transfer_method ?? 'tool_file',
    file_type: file?.file_type ?? payload.file_type,
    operation_id: file?.operation_id ?? payload.operation_id,
    correlation_id: file?.correlation_id ?? payload.correlation_id,
    asset_operation_audit: file?.asset_operation_audit ?? payload.asset_operation_audit,
    created_at: file?.created_at ?? payload.created_at ?? Math.floor(Date.now() / 1000),
  };
}

export function applySkillArtifactCreatedState(
  current: AIChatControllerState,
  payload: AIChatSkillArtifactCreatedEventData,
  eventId?: string | null
): AIChatControllerState {
  const nextGeneratedFile = normalizeSkillArtifactFile(payload);
  if (!nextGeneratedFile) {
    return current;
  }

  const previousStreaming = current.streamingByMessageId[payload.message_id];
  if (isStaleAIChatStreamEvent(eventId, previousStreaming?.last_event_id)) {
    return current;
  }
  const messages = current.messagesByConversation[payload.conversation_id] ?? [];
  const nextMessages = messages.map(message => {
    if (message.id !== payload.message_id) {
      return message;
    }

    const generatedFiles = upsertGeneratedFile(
      message.metadata?.generated_files ?? [],
      nextGeneratedFile
    );

    return {
      ...message,
      metadata: {
        ...(message.metadata ?? {}),
        generated_file_count: generatedFiles.length,
        generated_files: generatedFiles,
      },
      updated_at: Math.floor(Date.now() / 1000),
    };
  });
  return {
    ...current,
    messagesByConversation: {
      ...current.messagesByConversation,
      [payload.conversation_id]: nextMessages,
    },
    streamingByMessageId: previousStreaming
      ? {
          ...current.streamingByMessageId,
          [payload.message_id]: {
            ...previousStreaming,
            last_event_id: eventId ?? previousStreaming.last_event_id,
          },
        }
      : current.streamingByMessageId,
  };
}

export function applyFileParseStartState(
  current: AIChatControllerState,
  payload: AIChatFileParseStartEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            content_status: file.content_status ?? 'pending',
            parse_status: 'parsing',
            error: undefined,
          }
        : { ...file, id: payload.file_id }
  );
}

export function applyFileParseEndState(
  current: AIChatControllerState,
  payload: AIChatFileParseEndEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            content_status: payload.content_status,
            content_chars: payload.content_chars,
            from_cache: payload.from_cache,
            vision_detail: payload.vision_detail ?? null,
            filtered_reason: payload.filtered_reason ?? null,
            parse_status: 'completed',
            error: undefined,
          }
        : { ...file, id: payload.file_id }
  );
}

export function applyFileParseErrorState(
  current: AIChatControllerState,
  payload: AIChatFileParseErrorEventData,
  eventId?: string | null
): AIChatControllerState {
  return updateMessageFileMetadata(
    current,
    payload.conversation_id,
    payload.message_id,
    payload.file_id,
    payload.name,
    eventId,
    file =>
      file.id === payload.file_id
        ? {
            ...file,
            id: payload.file_id,
            name: payload.name || file.name,
            kind: payload.kind,
            parse_status: 'error',
            error: payload.message,
          }
        : { ...file, id: payload.file_id }
  );
}
