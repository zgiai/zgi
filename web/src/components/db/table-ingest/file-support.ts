'use client';

export const TABLE_INGEST_DOCUMENT_EXTENSIONS = [
  'txt',
  'md',
  'mdx',
  'markdown',
  'pdf',
  'html',
  'htm',
  'xlsx',
  'xls',
  'doc',
  'docx',
  'csv',
  'eml',
  'msg',
  'xml',
  'epub',
] as const;

export const TABLE_INGEST_IMAGE_EXTENSIONS = [
  'jpg',
  'jpeg',
  'png',
  'webp',
  'gif',
  'bmp',
  'tif',
  'tiff',
] as const;

export const TABLE_INGEST_ALL_EXTENSIONS = [
  ...TABLE_INGEST_DOCUMENT_EXTENSIONS,
  ...TABLE_INGEST_IMAGE_EXTENSIONS,
] as const;

interface IngestFileLike {
  extension?: string | null;
  mime_type?: string | null;
}

export function normalizeIngestExtension(extension?: string | null): string {
  return (extension ?? '').trim().toLowerCase().replace(/^\./, '');
}

export function isTableIngestImageFile(file: IngestFileLike): boolean {
  const extension = normalizeIngestExtension(file.extension);
  return (
    TABLE_INGEST_IMAGE_EXTENSIONS.includes(
      extension as (typeof TABLE_INGEST_IMAGE_EXTENSIONS)[number]
    ) || Boolean(file.mime_type?.toLowerCase().startsWith('image/'))
  );
}

export function isTableIngestAllowedFile(file: IngestFileLike, allowImages: boolean): boolean {
  const extension = normalizeIngestExtension(file.extension);
  const allowed: readonly string[] = allowImages
    ? TABLE_INGEST_ALL_EXTENSIONS
    : TABLE_INGEST_DOCUMENT_EXTENSIONS;

  return allowed.includes(extension);
}

export function isTableIngestSupportedFile(file: IngestFileLike): boolean {
  return isTableIngestAllowedFile(file, true);
}
