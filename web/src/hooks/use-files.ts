'use client';

import React from 'react';
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { fileManageService } from '@/services/file-manage.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { ApiResponseData } from '@/services/types/common';
import type {
  AllFilesResponse,
  FileItem,
  StorageUsage,
  FileFoldersResponse,
  FileFolder,
  UploadFileRequest,
  UploadFileResponse,
  CreateFolderRequest,
  CreateFolderResponse,
  UpdateFolderRequest,
  UpdateFolderResponse,
  MoveFolderRequest,
  CreateTextFileRequest,
  CreateTextFileResponse,
  GetAllFilesRequest,
} from '@/services/types/file';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

export const FILES_QUERY_KEY = 'all-files';

export const getAllFilesKey = (
  limit: string = '20',
  page: number,
  keyword?: string,
  sort?: string
) => [
  FILES_QUERY_KEY,
  'paginated',
  { limit, page, keyword: keyword?.trim() || '', sort: sort?.trim() || '' },
];

export const STORAGE_USAGE_KEY = 'storage-usage';
export const FILE_FOLDERS_KEY = 'file-folders';
export const UPLOAD_FILE_KEY = 'upload-file';
export const CREATE_FOLDER_KEY = 'create-folder';
export const UPDATE_FOLDER_KEY = 'update-folder';
export const MOVE_FOLDER_KEY = 'move-folder';
export const DELETE_FOLDER_KEY = 'delete-folder';
export const CREATE_TEXT_FILE_KEY = 'create-text-file';

const isFileFolderListQuery = (queryKey: readonly unknown[]) =>
  queryKey[0] === FILE_FOLDERS_KEY && queryKey[1] !== 'detail';

interface FilesQueryScope {
  workspace_id?: string;
  workspaceId?: string;
}

const isFilesQueryScope = (value: unknown): value is FilesQueryScope =>
  typeof value === 'object' && value !== null;

const getWorkspaceIdFromFilesQueryKey = (queryKey: readonly unknown[]) => {
  const params = queryKey.find(isFilesQueryScope);
  return params?.workspace_id || params?.workspaceId;
};

export interface UseAllFilesOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
  keyword?: string;
  sort?: string;
  extension?: string;
  workspaceId?: string;
}

export type FileCategory = 'all' | 'uploaded' | 'favorites' | '' | string;

export interface UseFilesOptions extends UseAllFilesOptions {
  category?: FileCategory;
}

/**
 * Paginated files hook with page navigation support
 *
 * @param limit - Number of items per page (default: 20)
 * @param options - Query options including keyword, sort, and React Query config
 */
export function useAllFiles(
  limit: string = '20',
  options: UseAllFilesOptions = {}
): {
  files: FileItem[];
  currentPage: number;
  totalPages: number;
  total: number;
  hasMore: boolean;
  hasPreviousPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  goToNextPage: () => void;
  goToPreviousPage: () => void;
  goToPage: (page: number) => void;
} {
  const t = useT('files');
  const [currentPage, setCurrentPage] = React.useState(1);
  const {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
    keyword,
    sort,
    extension,
  } = options;

  // Reset to page 1 when search keyword or sort changes
  React.useEffect(() => {
    setCurrentPage(1);
  }, [keyword, sort, extension]);

  const { data, isLoading, isFetching, error } = useQuery<ApiResponseData<AllFilesResponse>>({
    queryKey: getAllFilesKey(limit, currentPage, keyword, sort),
    queryFn: async () => {
      return fileManageService.getAllFiles({
        page: String(currentPage),
        limit,
        keyword,
        sort,
        extension,
      });
    },
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('toast.loadFilesError');
    toast.error(message);
  }

  const files: FileItem[] = data?.data?.data ?? [];
  const total = data?.data?.total ?? 0;
  const hasMore = data?.data?.has_more ?? false;
  const hasPreviousPage = currentPage > 1;
  const totalPages = Math.ceil(total / Number(limit));

  // Navigation functions
  const goToNextPage = React.useCallback(() => {
    if (hasMore) {
      setCurrentPage(prev => prev + 1);
    }
  }, [hasMore]);

  const goToPreviousPage = React.useCallback(() => {
    if (hasPreviousPage) {
      setCurrentPage(prev => prev - 1);
    }
  }, [hasPreviousPage]);

  const goToPage = React.useCallback(
    (page: number) => {
      if (page >= 1 && page <= totalPages) {
        setCurrentPage(page);
      }
    },
    [totalPages]
  );

  return {
    files,
    currentPage,
    totalPages,
    total,
    hasMore,
    hasPreviousPage,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    goToNextPage,
    goToPreviousPage,
    goToPage,
  };
}

/**
 * Get query key based on category
 */
const getFilesKey = (
  category: FileCategory,
  limit: string,
  page: number,
  keyword?: string,
  sort?: string,
  extension?: string,
  workspaceId?: string
) => {
  return [
    FILES_QUERY_KEY,
    category,
    'paginated',
    {
      limit,
      page,
      keyword: keyword?.trim() || '',
      sort: sort?.trim() || '',
      extension: extension || '',
      workspaceId: workspaceId || '',
    },
  ];
};

/**
 * Get service method based on category
 */
const getServiceMethod = (category: FileCategory) => {
  switch (category) {
    case 'all':
      return fileManageService.getAllFiles.bind(fileManageService);
    case 'needs_action':
      return fileManageService.getAllFiles.bind(fileManageService);
    case 'uploaded':
      return fileManageService.getRecentFiles.bind(fileManageService);
    // case 'favorites': // TODO: Temporarily disabled, may restore later
    //   return fileManageService.getFavoriteFiles.bind(fileManageService);
    case 'default':
      // Default folder: call getFolderFiles without folder_id
      return fileManageService.getFolderFiles.bind(fileManageService);
    default:
      // For specific folder IDs, call getFolderFiles with folder_id param
      return (params: GetAllFilesRequest) =>
        fileManageService.getFolderFiles({ ...params, folder_id: category });
  }
};

/**
 * Paginated files hook with category support
 * Supports different file categories and folders
 *
 * @param limit - Number of items per page (default: 20)
 * @param options - Query options including category, keyword, sort, and React Query config
 */
export function useFiles(
  limit: string = '20',
  options: UseFilesOptions = {}
): {
  files: FileItem[];
  currentPage: number;
  totalPages: number;
  total: number;
  hasMore: boolean;
  hasPreviousPage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  error: string | null;
  goToNextPage: () => void;
  goToPreviousPage: () => void;
  goToPage: (page: number) => void;
  reload: () => Promise<void>;
} {
  const t = useT('files');
  const [currentPage, setCurrentPage] = React.useState(1);
  const queryClient = useQueryClient();
  const {
    enabled = true,
    staleTime = 5 * 60 * 1000,
    gcTime = 30 * 60 * 1000,
    refetchOnWindowFocus = false,
    refetchInterval = false,
    keyword,
    sort,
    category = 'all',
    extension,
    workspaceId,
  } = options;

  // Reset to page 1 when search keyword, sort, category, or extension changes
  React.useEffect(() => {
    setCurrentPage(1);
  }, [keyword, sort, category, extension, workspaceId]);

  const serviceMethod = getServiceMethod(category);

  const { data, isLoading, isFetching, error } = useQuery<ApiResponseData<AllFilesResponse>>({
    queryKey: getFilesKey(category, limit, currentPage, keyword, sort, extension, workspaceId),
    queryFn: async () => {
      return serviceMethod({
        page: String(currentPage),
        limit,
        keyword,
        sort,
        extension,
        processing_status: category === 'needs_action' ? 'parse_failed' : undefined,
        workspace_id: workspaceId,
      });
    },
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('toast.loadFilesError');
    toast.error(message);
  }

  const files: FileItem[] = data?.data?.data ?? [];
  const total = data?.data?.total ?? 0;
  const hasMore = data?.data?.has_more ?? false;
  const hasPreviousPage = currentPage > 1;
  const totalPages = Math.ceil(total / Number(limit));

  // Navigation functions
  const goToNextPage = React.useCallback(() => {
    if (hasMore) {
      setCurrentPage(prev => prev + 1);
    }
  }, [hasMore]);

  const goToPreviousPage = React.useCallback(() => {
    if (hasPreviousPage) {
      setCurrentPage(prev => prev - 1);
    }
  }, [hasPreviousPage]);

  const goToPage = React.useCallback(
    (page: number) => {
      if (page >= 1 && page <= totalPages) {
        setCurrentPage(page);
      }
    },
    [totalPages]
  );

  // Reload all cached pages for the current category/filter
  const reload = React.useCallback(async () => {
    await queryClient.invalidateQueries({
      queryKey: [FILES_QUERY_KEY, category, 'paginated'],
    });
  }, [queryClient, category]);

  return {
    files,
    currentPage,
    totalPages,
    total,
    hasMore,
    hasPreviousPage,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    goToNextPage,
    goToPreviousPage,
    goToPage,
    reload,
  };
}

/**
 * Storage usage hook
 */
export function useStorageUsage(): {
  used: number;
  total: number;
  isLoading: boolean;
  error: string | null;
} {
  const t = useT('files');

  const { data, isLoading, error } = useQuery<ApiResponseData<StorageUsage>>({
    queryKey: [STORAGE_USAGE_KEY],
    queryFn: async () => {
      return fileManageService.getStorageUsage();
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 30 * 60 * 1000, // 30 minutes
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('toast.storageUsageError');
    toast.error(message);
  }

  const storageData = data?.data;

  return {
    used: storageData?.used || 0,
    total: storageData?.total || 0,
    isLoading,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useDownloadFile - download file functionality                       */
/* -------------------------------------------------------------------------- */

/**
 * File download hook
 */
export function useDownloadFile(): {
  downloadFile: (fileId: string, fileName: string) => Promise<void>;
  isDownloading: boolean;
} {
  const t = useT('files');

  const { mutateAsync: downloadFileMutation, isPending: isDownloading } = useMutation({
    mutationFn: async ({ fileId, fileName }: { fileId: string; fileName: string }) => {
      const blob = await fileManageService.downloadFile(fileId);

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = fileName;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    },
    onSuccess: () => {
      toast.success(t('toast.downloadSuccess'));
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.downloadError');
      toast.error(message);
    },
  });

  const downloadFile = async (fileId: string, fileName: string) => {
    await downloadFileMutation({ fileId, fileName });
  };

  return {
    downloadFile,
    isDownloading,
  };
}

/**
 * Custom error for files with associations
 */
export class FileAssociationError extends Error {
  fileNames: string[];

  constructor(fileNames: string[]) {
    super('FILES_HAVE_ASSOCIATIONS');
    this.name = 'FileAssociationError';
    this.fileNames = fileNames;
  }
}

/**
 * Delete files hook
 */
export function useDeleteFiles(): {
  deleteFiles: (fileIds: string[], files: FileItem[]) => Promise<void>;
  isDeleting: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: deleteFilesMutation, isPending: isDeleting } = useMutation({
    mutationFn: async ({ fileIds, files }: { fileIds: string[]; files: FileItem[] }) => {
      // Check if any files have associations
      const filesWithAssociations = files.filter(
        file => fileIds.includes(file.id) && file.related_count > 0
      );

      if (filesWithAssociations.length > 0) {
        const fileNames = filesWithAssociations.map(f => f.name);
        throw new FileAssociationError(fileNames);
      }
      await fileManageService.deleteFiles(fileIds);
    },
    onSuccess: () => {
      toast.success(t('toast.deleteSuccess'));
      // Invalidate both paginated and basic lists that are either unfiltered or belong to current workspace
      queryClient.invalidateQueries({
        queryKey: [FILES_QUERY_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILES_QUERY_KEY) return false;
          const wId = getWorkspaceIdFromFilesQueryKey(key);
          return !wId || wId === currentWorkspaceId;
        },
      });
    },
    onError: error => {
      // Don't show toast for association errors - let the component handle it
      if (error instanceof FileAssociationError) {
        return;
      }
      const message = (error as { message?: string }).message ?? t('toast.deleteError');
      toast.error(message);
    },
  });

  const deleteFiles = async (fileIds: string[], files: FileItem[]) => {
    await deleteFilesMutation({ fileIds, files });
  };

  return {
    deleteFiles,
    isDeleting,
  };
}

/**
 * Add file to favorites hook with toast feedback and list invalidation
 */
export function useAddFileFavorite(): {
  addFavorite: (fileId: string) => Promise<void>;
  isAdding: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: addFavoriteMutation, isPending: isAdding } = useMutation({
    mutationFn: async (fileId: string) => {
      await fileManageService.addFileFavorite(fileId);
    },
    onSuccess: () => {
      toast.success(t('toast.addFavoriteSuccess'));
      // Invalidate if unfiltered or current workspace
      queryClient.invalidateQueries({
        queryKey: [FILES_QUERY_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILES_QUERY_KEY) return false;
          const tId = getWorkspaceIdFromFilesQueryKey(key);
          return !tId || tId === currentWorkspaceId;
        },
      });
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('toast.addFavoriteError');
      toast.error(message);
    },
  });

  const addFavorite = async (fileId: string) => {
    await addFavoriteMutation(fileId);
  };

  return {
    addFavorite,
    isAdding,
  };
}

/**
 * Remove file from favorites hook with toast feedback and list invalidation
 */
export function useRemoveFileFavorite(): {
  removeFavorite: (fileId: string) => Promise<void>;
  isRemoving: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: removeFavoriteMutation, isPending: isRemoving } = useMutation({
    mutationFn: async (fileId: string) => {
      await fileManageService.removeFileFavorite(fileId);
    },
    onSuccess: () => {
      toast.success(t('toast.removeFavoriteSuccess'));
      // Invalidate if unfiltered or current workspace
      queryClient.invalidateQueries({
        queryKey: [FILES_QUERY_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILES_QUERY_KEY) return false;
          const tId = getWorkspaceIdFromFilesQueryKey(key);
          return !tId || tId === currentWorkspaceId;
        },
      });
    },
    onError: (error: unknown) => {
      const message = (error as { message?: string }).message ?? t('toast.removeFavoriteError');
      toast.error(message);
    },
  });

  const removeFavorite = async (fileId: string) => {
    await removeFavoriteMutation(fileId);
  };

  return {
    removeFavorite,
    isRemoving,
  };
}

/**
 * File folders hook - fetches all file folders
 */
export function useFileFolders(
  workspaceId?: string,
  options: { enabled?: boolean } = {}
): {
  folders: FileFolder[];
  isLoading: boolean;
  error: string | null;
  refetch: () => void;
} {
  const t = useT('files');

  const { data, isLoading, error, refetch } = useQuery<ApiResponseData<FileFoldersResponse>>({
    queryKey: [FILE_FOLDERS_KEY, workspaceId],
    queryFn: async () => {
      return fileManageService.getFileFolders(workspaceId);
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 30 * 60 * 1000, // 30 minutes
    refetchOnWindowFocus: false,
    retry: false,
    enabled: options.enabled ?? true,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('toast.foldersLoadError');
    toast.error(message);
  }

  const folders: FileFolder[] = data?.data?.data ?? [];

  return {
    folders,
    isLoading,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}

/**
 * Child folders hook - fetches child folders of a specific parent folder
 * @param parentId - Parent folder ID (undefined to disable query)
 */
export function useChildFolders(
  parentId?: string,
  workspaceId?: string
): {
  folders: FileFolder[];
  isLoading: boolean;
  error: string | null;
  refetch: () => void;
} {
  const t = useT('files');

  const { data, isLoading, error, refetch } = useQuery<ApiResponseData<FileFoldersResponse>>({
    queryKey: [FILE_FOLDERS_KEY, 'children', parentId, workspaceId],
    queryFn: async () => {
      return fileManageService.getChildFolders(parentId, workspaceId);
    },
    enabled: parentId !== undefined,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 30 * 60 * 1000, // 30 minutes
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Show toast on error
  if (error) {
    const message = (error as { message?: string }).message ?? t('toast.childFoldersLoadError');
    toast.error(message);
  }

  const folders: FileFolder[] = data?.data?.data ?? [];

  return {
    folders,
    isLoading,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    refetch,
  };
}

/* -------------------------------------------------------------------------- */
/* Hook: useUploadFileToFolder - upload file to folder                       */
/* -------------------------------------------------------------------------- */

/**
 * Upload file to folder hook with optimistic updates
 */
export function useUploadFileToFolder(): {
  uploadFile: (data: UploadFileRequest) => Promise<UploadFileResponse>;
  isUploading: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: uploadFileMutation, isPending: isUploading } = useMutation({
    mutationFn: async (data: UploadFileRequest) => {
      const response = await fileManageService.uploadFileToFolder(data);
      return response.data;
    },
    onSuccess: () => {
      toast.success(t('toast.uploadSuccess'));
      // Invalidate if unfiltered or current workspace
      queryClient.invalidateQueries({
        queryKey: [FILES_QUERY_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILES_QUERY_KEY) return false;
          const wId = getWorkspaceIdFromFilesQueryKey(key);
          return !wId || wId === currentWorkspaceId;
        },
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.uploadError');
      toast.error(message);
    },
  });

  const uploadFile = async (data: UploadFileRequest) => {
    return await uploadFileMutation(data);
  };

  return {
    uploadFile,
    isUploading,
  };
}

/**
 * Create folder hook with optimistic updates
 */
export function useCreateFolder(): {
  createFolder: (data: CreateFolderRequest) => Promise<CreateFolderResponse>;
  isCreating: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: createFolderMutation, isPending: isCreating } = useMutation({
    mutationFn: async (data: CreateFolderRequest) => {
      const response = await fileManageService.createFolder(data);
      return response.data;
    },
    onSuccess: (_, variables) => {
      toast.success(t('toast.createFolderSuccess'));
      const targetWorkspaceId = variables.workspace_id || currentWorkspaceId;
      queryClient.invalidateQueries({
        queryKey: [FILE_FOLDERS_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILE_FOLDERS_KEY) return false;
          if (key[1] === 'detail') return false;

          if (key[1] === 'children') {
            const parentId = key[2];
            const workspaceId = key[3];
            return parentId === variables.parent_id && (!workspaceId || workspaceId === targetWorkspaceId);
          }

          const workspaceId = key[1];
          return !workspaceId || workspaceId === targetWorkspaceId;
        },
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.createFolderError');
      toast.error(message);
    },
  });

  const createFolder = async (data: CreateFolderRequest) => {
    return await createFolderMutation(data);
  };

  return {
    createFolder,
    isCreating,
  };
}

/**
 * Update folder hook with optimistic updates
 */
export function useUpdateFolder(): {
  updateFolder: (folderId: string, data: UpdateFolderRequest) => Promise<UpdateFolderResponse>;
  isUpdating: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();

  const { mutateAsync: updateFolderMutation, isPending: isUpdating } = useMutation({
    mutationFn: async ({ folderId, data }: { folderId: string; data: UpdateFolderRequest }) => {
      const response = await fileManageService.updateFolder(folderId, data);
      return response.data;
    },
    onSuccess: (_, { folderId }) => {
      toast.success(t('toast.updateFolderSuccess'));
      if (folderId) {
        queryClient.invalidateQueries({ queryKey: [FILE_FOLDERS_KEY, 'detail', folderId] });
      }
      // Refresh root and nested folder lists so renamed or moved child folders update in the sidebar.
      queryClient.invalidateQueries({
        queryKey: [FILE_FOLDERS_KEY],
        predicate: query => isFileFolderListQuery(query.queryKey),
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.updateFolderError');
      toast.error(message);
    },
  });

  const updateFolder = async (folderId: string, data: UpdateFolderRequest) => {
    return await updateFolderMutation({ folderId, data });
  };

  return {
    updateFolder,
    isUpdating,
  };
}

/**
 * Move folder hook
 */
export function useMoveFolder(): {
  moveFolder: (data: MoveFolderRequest) => Promise<void>;
  isMoving: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();

  const { mutateAsync: moveFolderMutation, isPending: isMoving } = useMutation({
    mutationFn: async (data: MoveFolderRequest) => {
      await fileManageService.moveFolder(data);
    },
    onSuccess: () => {
      toast.success(t('toast.moveFolderSuccess'));
      queryClient.invalidateQueries({
        queryKey: [FILE_FOLDERS_KEY],
        predicate: query => isFileFolderListQuery(query.queryKey),
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.moveFolderError');
      toast.error(message);
    },
  });

  const moveFolder = async (data: MoveFolderRequest) => {
    return await moveFolderMutation(data);
  };

  return {
    moveFolder,
    isMoving,
  };
}

/**
 * Delete folder hook with optimistic updates
 */
export function useDeleteFolder(): {
  deleteFolder: (folderId: string) => Promise<void>;
  isDeleting: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();

  const { mutateAsync: deleteFolderMutation, isPending: isDeleting } = useMutation({
    mutationFn: async (folderId: string) => {
      await fileManageService.deleteFolder(folderId);
    },
    onSuccess: (_, folderId) => {
      toast.success(t('toast.deleteFolderSuccess'));
      if (folderId) {
        queryClient.removeQueries({ queryKey: [FILE_FOLDERS_KEY, 'detail', folderId] });
      }
      // Refresh root and nested folder lists so deleted child folders disappear from the sidebar.
      queryClient.invalidateQueries({
        queryKey: [FILE_FOLDERS_KEY],
        predicate: query => isFileFolderListQuery(query.queryKey),
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.deleteFolderError');
      toast.error(message);
    },
  });

  const deleteFolder = async (folderId: string) => {
    return await deleteFolderMutation(folderId);
  };

  return {
    deleteFolder,
    isDeleting,
  };
}

/**
 * Create text file hook with optimistic updates
 */
export function useCreateTextFile(): {
  createTextFile: (data: CreateTextFileRequest) => Promise<CreateTextFileResponse>;
  isCreating: boolean;
} {
  const t = useT('files');
  const queryClient = useQueryClient();
  const currentWorkspaceId = useCurrentWorkspace()?.id;

  const { mutateAsync: createTextFileMutation, isPending: isCreating } = useMutation({
    mutationFn: async (data: CreateTextFileRequest) => {
      const response = await fileManageService.createTextFile(data);
      return response.data;
    },
    onSuccess: (_, variables) => {
      toast.success(t('toast.createTextFileSuccess'));
      const targetWorkspaceId = variables.workspace_id || currentWorkspaceId;
      // Invalidate if unfiltered or current workspace
      queryClient.invalidateQueries({
        queryKey: [FILES_QUERY_KEY],
        predicate: query => {
          const key = query.queryKey;
          if (key[0] !== FILES_QUERY_KEY) return false;
          const wId = getWorkspaceIdFromFilesQueryKey(key);
          return !wId || wId === targetWorkspaceId;
        },
      });
    },
    onError: error => {
      const message = (error as { message?: string }).message ?? t('toast.createTextFileError');
      toast.error(message);
    },
  });

  const createTextFile = async (data: CreateTextFileRequest) => {
    return await createTextFileMutation(data);
  };

  return {
    createTextFile,
    isCreating,
  };
}
