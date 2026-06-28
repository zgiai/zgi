import type { AIChatPageContextItem } from '@/components/aichat/page-context';
import type { FileItem } from '@/services/types/file';
import type { Workspace } from '@/store/workspace-store';

export interface FilesAIChatPresentation {
  homeTitle: string;
  homeDescription: string;
  inputPlaceholder: string;
  suggestions: string[];
}

export type FilesAIChatQueryStatus = 'loading' | 'ready' | 'error';

export interface FilesAIChatContextSnapshot {
  files: FileItem[];
  selectedFileIds: string[];
  currentPage: number;
  totalPages: number;
  total: number;
  pageSize: number;
  sort: string;
  activeCategory: string;
  searchValue: string;
  extensionParam?: string;
  currentWorkspace: Workspace | null;
  isOrganizationMode: boolean;
  activeFolderName?: string;
  contextReady: boolean;
  queryStatus: FilesAIChatQueryStatus;
  canManage: boolean;
  canUpload: boolean;
  presentation?: FilesAIChatPresentation;
}

export type FilesAIChatContextItem = AIChatPageContextItem;
