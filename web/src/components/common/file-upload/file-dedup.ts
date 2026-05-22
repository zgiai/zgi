import type { UploadedFile } from '@/services/types/dataset';
import type { FileItem } from '@/services/types/file';

interface ExistingUploadItem {
  id: string;
  file: File;
  contentHash?: string;
  serverFile?: UploadedFile;
}

function normalizeHash(hash: unknown): string {
  return typeof hash === 'string' ? hash.trim().toLowerCase() : '';
}

export async function calculateFileHash(file: File): Promise<string> {
  const buffer = await file.arrayBuffer();
  const digest = await crypto.subtle.digest('SHA-256', buffer);
  return Array.from(new Uint8Array(digest))
    .map(byte => byte.toString(16).padStart(2, '0'))
    .join('');
}

export function getFileFallbackKey(file: File): string {
  return [file.name, file.size, file.type, file.lastModified].join('|');
}

export function getExistingFileKeys(items: ExistingUploadItem[]): Set<string> {
  const keys = new Set<string>();
  items.forEach(item => {
    const itemHash = normalizeHash(item.contentHash) || normalizeHash(item.serverFile?.hash);
    if (itemHash) keys.add(`hash:${itemHash}`);
    if (item.serverFile?.id) keys.add(`id:${item.serverFile.id}`);
    keys.add(`local:${getFileFallbackKey(item.file)}`);
  });
  return keys;
}

export function getUploadedFileKeys(file: UploadedFile | FileItem): string[] {
  const keys: string[] = [];
  const hash = normalizeHash(file.hash);
  if (hash) keys.push(`hash:${hash}`);
  if (file.id) keys.push(`id:${file.id}`);
  return keys;
}

export function hasAnyFileKey(existingKeys: Set<string>, keys: string[]): boolean {
  return keys.some(key => existingKeys.has(key));
}
