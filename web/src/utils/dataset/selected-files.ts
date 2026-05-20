// Utilities for persisting selected upload files between pages
// All comments are in English. Types are strict.

import type { FileItem } from '@/services/types/file';
import type { UploadedFile } from '@/services/types/dataset';

const KEY_PREFIX = 'zgi:selected-upload-files:';

function buildKey(datasetId: string): string {
  return `${KEY_PREFIX}${datasetId}`;
}

export function mapFileItemToUploadedFile(file: FileItem): UploadedFile {
  return {
    id: file.id,
    name: file.name,
    size: file.size,
    extension: file.extension,
    mime_type: file.mime_type,
    hash: file.hash || '',
    created_by: file.created_by,
    created_at: file.created_at,
    url: '',
  };
}

export function saveSelectedUploadFiles(datasetId: string, files: FileItem[]): void {
  try {
    const mapped: UploadedFile[] = files.map(mapFileItemToUploadedFile);
    sessionStorage.setItem(buildKey(datasetId), JSON.stringify(mapped));
  } catch (_e) {
    // ignore storage errors silently
  }
}

export function loadSelectedUploadFiles(datasetId: string): UploadedFile[] {
  try {
    const raw = sessionStorage.getItem(buildKey(datasetId));
    if (!raw) return [];
    const parsed = JSON.parse(raw) as UploadedFile[];
    // Ensure minimal validation of shape
    return Array.isArray(parsed) ? parsed.filter(f => !!f && typeof f.id === 'string') : [];
  } catch (_e) {
    return [];
  }
}

export function clearSelectedUploadFiles(datasetId: string): void {
  try {
    sessionStorage.removeItem(buildKey(datasetId));
  } catch (_e) {
    // ignore
  }
}

// Save already mapped UploadedFile list
export function saveUploadedFiles(datasetId: string, files: UploadedFile[]): void {
  try {
    sessionStorage.setItem(buildKey(datasetId), JSON.stringify(files));
  } catch (_e) {
    // ignore
  }
}

// Key prefix for full FileItem storage
const FILE_ITEM_KEY_PREFIX = 'zgi:selected-file-items:';

function buildFileItemKey(datasetId: string): string {
  return `${FILE_ITEM_KEY_PREFIX}${datasetId}`;
}

// Save full FileItem[] for cross-page selection support
export function saveSelectedFileItems(datasetId: string, files: FileItem[]): void {
  try {
    sessionStorage.setItem(buildFileItemKey(datasetId), JSON.stringify(files));
  } catch (_e) {
    // ignore storage errors silently
  }
}

// Load full FileItem[] for dialog initialization
export function loadSelectedFileItems(datasetId: string): FileItem[] {
  try {
    const raw = sessionStorage.getItem(buildFileItemKey(datasetId));
    if (!raw) return [];
    const parsed = JSON.parse(raw) as FileItem[];
    return Array.isArray(parsed) ? parsed.filter(f => !!f && typeof f.id === 'string') : [];
  } catch (_e) {
    return [];
  }
}

// Clear full FileItem storage
export function clearSelectedFileItems(datasetId: string): void {
  try {
    sessionStorage.removeItem(buildFileItemKey(datasetId));
  } catch (_e) {
    // ignore
  }
}
