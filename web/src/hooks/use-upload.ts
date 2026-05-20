'use client';

// Upload hooks powered by React Query
// All comments are in English for clarity and maintainability

import { useCallback, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { uploadService } from '@/services';
import type {
  UploadResponse,
  MultipleUploadResponse,
  UploadConfig,
} from '@/services/upload.service';
import { type SupportedFileTypesResponse, fileService } from '@/services/file.service';

/* -------------------------------------------------------------------------- */
/* Query-key helpers                                                          */
/* -------------------------------------------------------------------------- */

const UPLOAD_QUERY_KEY = 'upload';

const getSupportedTypesKey = () => [UPLOAD_QUERY_KEY, 'supported-types'];
const getUploadConfigKey = () => [UPLOAD_QUERY_KEY, 'config'];

/* -------------------------------------------------------------------------- */
/* Hook: useUploadConfig                                                       */
/* -------------------------------------------------------------------------- */

export function useUploadConfig(options?: { enabled?: boolean }) {
  return useQuery<UploadConfig, Error>({
    queryKey: getUploadConfigKey(),
    queryFn: () => uploadService.getConfig(),
    staleTime: 0,
    gcTime: 48 * 60 * 60 * 1000,
    enabled: options?.enabled,
    refetchOnMount: 'always',
    refetchOnWindowFocus: true,
  });
}

/* -------------------------------------------------------------------------- */
/* Hook: useSupportedFileTypes                                                 */
/* -------------------------------------------------------------------------- */

export interface UseSupportedFileTypesOptions {
  enabled?: boolean;
  staleTime?: number;
  gcTime?: number;
  refetchOnWindowFocus?: boolean;
  refetchInterval?: number | false;
}

export function useSupportedFileTypes({
  enabled = true,
  staleTime = 24 * 60 * 60 * 1000, // 24h cache – this rarely changes
  gcTime = 48 * 60 * 60 * 1000,
  refetchOnWindowFocus = false,
  refetchInterval = false,
}: UseSupportedFileTypesOptions = {}) {
  const { data, isLoading, isFetching, error, refetch } = useQuery<
    SupportedFileTypesResponse,
    unknown
  >({
    queryKey: getSupportedTypesKey(),
    queryFn: () => fileService.getSupportedFileTypes(),
    enabled,
    staleTime,
    gcTime,
    refetchOnWindowFocus,
    refetchInterval,
    retry: false,
  });

  const reload = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return {
    supportedTypes: data ?? ({} as SupportedFileTypesResponse),
    raw: data,
    isLoading,
    isFetching,
    error: error ? ((error as { message?: string }).message ?? 'error') : null,
    reload,
  } as const;
}

/* -------------------------------------------------------------------------- */
/* Upload helpers without React Query                                          */
/* -------------------------------------------------------------------------- */

export function useUploadSingle(
  onSuccess?: (file: UploadResponse) => void,
  options: { workspace_id?: string; is_temporary?: boolean } = {}
) {
  const [isUploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState<UploadResponse | null>(null);

  const upload = useCallback(
    async (file: File) => {
      setUploading(true);
      setError(null);
      try {
        const res = await uploadService.uploadSingle(file, options);
        setResponse(res);
        onSuccess?.(res);
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Upload failed';
        setError(msg);
        throw err;
      } finally {
        setUploading(false);
      }
    },
    [onSuccess, options]
  );

  return { upload, isUploading, error, response } as const;
}

export function useUploadMultiple(
  onSuccess?: (result: MultipleUploadResponse) => void,
  options: { workspace_id?: string; is_temporary?: boolean } = {}
) {
  const [isUploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState<MultipleUploadResponse | null>(null);

  const upload = useCallback(
    async (files: File[]) => {
      setUploading(true);
      setError(null);
      try {
        const res = await uploadService.uploadMultiple(files, options);
        setResponse(res);
        onSuccess?.(res);
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Upload failed';
        setError(msg);
        throw err;
      } finally {
        setUploading(false);
      }
    },
    [onSuccess, options]
  );

  return { upload, isUploading, error, response } as const;
}

export function useUploadSingleWithProgress() {
  const [progress, setProgress] = useState(0);
  const [isUploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState<UploadResponse | null>(null);

  const upload = useCallback(async (file: File, options?: { is_temporary?: boolean }) => {
    setUploading(true);
    setError(null);
    try {
      const res = await uploadService.uploadSingle(file, {
        onProgress: setProgress,
        ...options,
      });
      setResponse(res);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Upload failed';
      setError(msg);
      throw err;
    } finally {
      setUploading(false);
      setProgress(0);
    }
  }, []);

  return { upload, isUploading, error, response, progress } as const;
}

export function useUploadMultipleWithProgress() {
  const [progressMap, setProgressMap] = useState<Record<number, number>>({});
  const [isUploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [response, setResponse] = useState<MultipleUploadResponse | null>(null);

  const upload = useCallback(async (files: File[], options?: { is_temporary?: boolean }) => {
    setUploading(true);
    setError(null);
    setProgressMap({});
    try {
      const res = await Promise.all(
        files.map((file, idx) =>
          uploadService.uploadSingle(file, {
            onProgress: p => setProgressMap(prev => ({ ...prev, [idx]: p })),
            ...options,
          })
        )
      );
      const result: MultipleUploadResponse = {
        files: res,
        success: res.length,
        failed: 0,
      };
      setResponse(result);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Upload failed';
      setError(msg);
      throw err;
    } finally {
      setUploading(false);
      setTimeout(() => setProgressMap({}), 2000);
    }
  }, []);

  const overallProgress =
    Object.values(progressMap).reduce((sum, p) => sum + p, 0) /
    (Object.keys(progressMap).length || 1);

  return { upload, isUploading, error, response, progressMap, overallProgress } as const;
}
