import { fileManageService } from '@/services/file-manage.service';
import type { FileFolder } from '@/services/types/file';

export const MAX_FILE_FOLDER_LEVEL = 3;
export const MAX_FILE_FOLDER_TREE_LEVEL = MAX_FILE_FOLDER_LEVEL - 1;
export const MAX_FILE_FOLDER_PARENT_LEVEL = MAX_FILE_FOLDER_LEVEL - 1;

export type FileFolderOption = FileFolder & { depth: number };

export async function loadFileFolderOptions(
  rootFolders: FileFolder[],
  workspaceId: string | undefined,
  maxDepth: number
): Promise<FileFolderOption[]> {
  const options: FileFolderOption[] = [];

  async function appendFolder(folder: FileFolder, depth: number) {
    options.push({ ...folder, depth });

    if (depth >= maxDepth) return;

    const response = await fileManageService.getChildFolders(folder.id, workspaceId);
    const childFolders = response.data?.data ?? [];

    for (const childFolder of childFolders) {
      await appendFolder(childFolder, depth + 1);
    }
  }

  for (const folder of rootFolders) {
    await appendFolder(folder, 1);
  }

  return options;
}

export function getFolderSubtreeHeight(folderId: string, options: FileFolderOption[]): number {
  const childIdsByParent = new Map<string, string[]>();

  for (const option of options) {
    if (!option.parent_id) continue;
    const childIds = childIdsByParent.get(option.parent_id) ?? [];
    childIds.push(option.id);
    childIdsByParent.set(option.parent_id, childIds);
  }

  function getHeight(currentId: string): number {
    const childIds = childIdsByParent.get(currentId) ?? [];
    if (childIds.length === 0) return 1;
    return 1 + Math.max(...childIds.map(getHeight));
  }

  return getHeight(folderId);
}

export function getDescendantFolderIds(folderId: string, options: FileFolderOption[]): Set<string> {
  const descendantIds = new Set<string>();
  const childIdsByParent = new Map<string, string[]>();

  for (const option of options) {
    if (!option.parent_id) continue;
    const childIds = childIdsByParent.get(option.parent_id) ?? [];
    childIds.push(option.id);
    childIdsByParent.set(option.parent_id, childIds);
  }

  function collect(currentId: string) {
    const childIds = childIdsByParent.get(currentId) ?? [];
    for (const childId of childIds) {
      descendantIds.add(childId);
      collect(childId);
    }
  }

  collect(folderId);
  return descendantIds;
}

export function getFileFolderAncestorIds(
  folders: Array<Pick<FileFolder, 'id' | 'parent_id'>>,
  folderId: string
) {
  const folderById = new Map(folders.map(folder => [folder.id, folder]));
  const ancestorIds: string[] = [];
  let current = folderById.get(folderId);

  while (current?.parent_id) {
    ancestorIds.push(current.parent_id);
    current = folderById.get(current.parent_id);
  }

  return ancestorIds;
}

export async function getFileFolderAncestorIdsByRequest(folderId: string) {
  const ancestorIds: string[] = [];
  let currentId = folderId;

  while (currentId) {
    const response = await fileManageService.getFileFolder(currentId);
    const parentId = response.data?.parent_id;

    if (!parentId) break;
    ancestorIds.push(parentId);
    currentId = parentId;
  }

  return ancestorIds;
}
