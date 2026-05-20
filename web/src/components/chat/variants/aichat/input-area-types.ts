import type { AIChatMessageFile } from '@/services/types/aichat';

export const AICHAT_ATTACHMENT_LIMIT = 5;

export const AICHAT_UPLOAD_EXTENSIONS = [
  'txt',
  'md',
  'markdown',
  'mdx',
  'pdf',
  'html',
  'htm',
  'doc',
  'docx',
  'xls',
  'xlsx',
  'csv',
  'eml',
  'msg',
  'xml',
  'epub',
] as const;

export const AICHAT_DOCUMENT_EXTENSIONS = [
  ...AICHAT_UPLOAD_EXTENSIONS,
  'ppt',
  'pptx',
  'json',
  'key',
  'numbers',
  'java',
  'sql',
] as const;

export type AIChatInputAttachmentStatus = 'uploading' | 'uploaded' | 'error';
export type AIChatInputAttachmentKind = 'document' | 'image';
export type AIChatAttachmentUploadKind = 'document' | 'image';

export interface AIChatInputAttachment {
  id: string;
  name: string;
  size: number;
  extension: string;
  kind: AIChatInputAttachmentKind;
  progress: number;
  status: AIChatInputAttachmentStatus;
  file?: AIChatMessageFile;
  previewUrl?: string;
  error?: string;
}
