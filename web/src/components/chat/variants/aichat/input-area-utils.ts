import type { ModelSelectorModelProps } from '@/components/common/model-selector';
import type {
  AIChatInputAttachmentKind,
  AIChatInputAttachment,
} from '@/components/chat/variants/aichat/input-area-types';
import type { AIChatMessageFile } from '@/services/types/aichat';
import type { FileItem } from '@/services/types/file';
import type { UploadResponse } from '@/services/upload.service';
import { generateClientId } from '@/utils/client-id';
import { IMAGE_EXTENSIONS } from '@/utils/file-helpers';

export interface ScopedTranslatorWithHas {
  (key: string, values?: Record<string, string | number | Date>): string;
  has?: (key: string) => boolean;
}

export function tWithFallback(
  t: ScopedTranslatorWithHas,
  key: string,
  fallbackKey: string,
  values?: Record<string, string | number | Date>
): string {
  try {
    if (typeof t.has === 'function' && !t.has(key)) {
      return t(fallbackKey, values);
    }

    return t(key, values);
  } catch {
    return t(fallbackKey, values);
  }
}

export function createAttachmentId(): string {
  return generateClientId('aichat-file');
}

export function formatFileSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) {
    return '0 B';
  }

  const units = ['B', 'KB', 'MB', 'GB'];
  let value = size;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value >= 10 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`;
}

export function toAIChatMessageFile(
  file: UploadResponse,
  kind: AIChatInputAttachmentKind = 'document'
): AIChatMessageFile {
  return {
    id: file.id,
    name: file.name,
    size: file.size,
    extension: file.extension,
    mime_type: file.mime_type,
    workspace_id: null,
    is_temporary: true,
    content_status: 'pending',
    content_chars: 0,
    content_preview: '',
    from_cache: false,
    kind,
    vision_detail: null,
    filtered_reason: null,
    parse_status: 'pending',
  };
}

export function getNormalizedExtension(value: string | null | undefined): string {
  return (value ?? '').toLowerCase().replace(/^\./, '');
}

export function isImageExtension(extension: string | null | undefined): boolean {
  return IMAGE_EXTENSIONS.includes(getNormalizedExtension(extension));
}

export function isVisionModel(model: ModelSelectorModelProps | null): boolean {
  return Boolean(model?.endpoints?.vision || model?.use_cases?.includes('vision'));
}

export function fileItemToAIChatMessageFile(file: FileItem): AIChatMessageFile {
  const kind: AIChatInputAttachmentKind = isImageExtension(file.extension) ? 'image' : 'document';
  return {
    id: file.id,
    name: file.name,
    size: file.size,
    extension: file.extension,
    mime_type: file.mime_type,
    workspace_id: file.workspace_id,
    is_temporary: false,
    content_status: 'pending',
    content_chars: 0,
    content_preview: '',
    from_cache: Boolean(file.content_text),
    kind,
    vision_detail: null,
    filtered_reason: null,
    parse_status: 'pending',
  };
}

export function getUploadedAIChatFiles(
  attachments: AIChatInputAttachment[]
): AIChatMessageFile[] {
  return attachments
    .filter(attachment => attachment.status === 'uploaded' && attachment.file)
    .map(attachment => attachment.file)
    .filter((file): file is AIChatMessageFile => Boolean(file));
}
