'use client';

import { useState, useCallback, useMemo } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type {
  CreateSegmentRequest,
  UpdateSegmentRequest,
  SegmentEnabledFilter,
  SegmentDetail,
} from '@/services/types/dataset';
import { DATASET_KEYS } from '@/hooks/query-keys';

interface UseDocumentSegmentsProps {
  datasetId: string;
  documentId: string;
  /** Whether to enable queries - set to false to skip all API calls */
  enabled?: boolean;
  /** Callback to refetch document after segment changes */
  onSegmentChange?: () => void;
}

/**
 * Hook for managing document segments (chunks)
 * Handles segment CRUD, pagination, filtering, and selection
 */
export function useDocumentSegments({
  datasetId,
  documentId,
  enabled = true,
  onSegmentChange,
}: UseDocumentSegmentsProps) {
  const t = useT('datasets');

  // Base enabled condition
  const queryEnabled = enabled && !!datasetId && !!documentId;

  // Segments state
  const [segmentsPage, setSegmentsPage] = useState(1);
  const [segmentsKeyword, setSegmentsKeyword] = useState('');
  const [segmentsEnabledFilter, setSegmentsEnabledFilter] = useState<SegmentEnabledFilter>('all');
  const [selectedSegments, setSelectedSegments] = useState<string[]>([]);

  // Segments query
  const {
    data: segmentsResponse,
    isLoading: isSegmentsLoading,
    refetch: refetchSegments,
  } = useQuery({
    queryKey: DATASET_KEYS.segmentList(datasetId, documentId, {
      page: segmentsPage,
      keyword: segmentsKeyword,
      enabled: segmentsEnabledFilter,
    }),
    queryFn: () =>
      datasetService.getSegments(datasetId, documentId, {
        page: segmentsPage,
        limit: 20,
        keyword: segmentsKeyword || undefined,
        enabled: segmentsEnabledFilter,
      }),
    staleTime: 10000, // 10 seconds
    enabled: queryEnabled,
  });

  const segments = useMemo<SegmentDetail[]>(
    () => segmentsResponse?.data.data || [],
    [segmentsResponse?.data.data]
  );
  const segmentsTotal = segmentsResponse?.data.total || 0;
  const segmentsTotalPages = segmentsResponse?.data.total_pages || 1;
  const segmentsPageSize = 20;

  // Create segment mutation
  const createSegmentMutation = useMutation({
    mutationFn: (data: CreateSegmentRequest) =>
      datasetService.createSegment(datasetId, documentId, data),
    onSuccess: () => {
      toast.success(t('segments.createSuccess'));
      refetchSegments();
      onSegmentChange?.();
    },
    onError: () => {
      toast.error(t('segments.createFailed'));
    },
  });

  // Update segment mutation
  const updateSegmentMutation = useMutation({
    mutationFn: ({ segmentId, data }: { segmentId: string; data: UpdateSegmentRequest }) =>
      datasetService.updateSegment(datasetId, documentId, segmentId, data),
    onSuccess: () => {
      toast.success(t('segments.updateSuccess'));
      refetchSegments();
    },
    onError: () => {
      toast.error(t('segments.updateFailed'));
    },
  });

  // Batch enable segments mutation
  const batchEnableSegmentsMutation = useMutation({
    mutationFn: (segmentIds: string[]) =>
      datasetService.batchEnableSegments(datasetId, documentId, segmentIds),
    onSuccess: (_data, segmentIds) => {
      toast.success(t('segments.batchEnableSuccess', { count: segmentIds.length }));
      refetchSegments();
      setSelectedSegments([]);
    },
    onError: () => {
      toast.error(t('segments.updateFailed'));
    },
  });

  // Batch disable segments mutation
  const batchDisableSegmentsMutation = useMutation({
    mutationFn: (segmentIds: string[]) =>
      datasetService.batchDisableSegments(datasetId, documentId, segmentIds),
    onSuccess: (_data, segmentIds) => {
      toast.success(t('segments.batchDisableSuccess', { count: segmentIds.length }));
      refetchSegments();
      setSelectedSegments([]);
    },
    onError: () => {
      toast.error(t('segments.updateFailed'));
    },
  });

  // Batch delete segments mutation
  const batchDeleteSegmentsMutation = useMutation({
    mutationFn: (segmentIds: string[]) =>
      datasetService.batchDeleteSegments(datasetId, documentId, segmentIds),
    onSuccess: (_data, segmentIds) => {
      toast.success(t('segments.batchDeleteSuccess', { count: segmentIds.length }));
      refetchSegments();
      onSegmentChange?.();
      setSelectedSegments([]);
    },
    onError: () => {
      toast.error(t('segments.deleteFailed'));
    },
  });

  // Actions - use mutateAsync to allow proper await in callers
  const createSegment = useCallback(
    (data: CreateSegmentRequest) => {
      return createSegmentMutation.mutateAsync(data);
    },
    [createSegmentMutation]
  );

  const updateSegment = useCallback(
    (segmentId: string, data: UpdateSegmentRequest) => {
      return updateSegmentMutation.mutateAsync({ segmentId, data });
    },
    [updateSegmentMutation]
  );

  const batchEnableSegments = useCallback(
    (segmentIds: string[]) => {
      batchEnableSegmentsMutation.mutate(segmentIds);
    },
    [batchEnableSegmentsMutation]
  );

  const batchDisableSegments = useCallback(
    (segmentIds: string[]) => {
      batchDisableSegmentsMutation.mutate(segmentIds);
    },
    [batchDisableSegmentsMutation]
  );

  const batchDeleteSegments = useCallback(
    (segmentIds: string[]) => {
      batchDeleteSegmentsMutation.mutate(segmentIds);
    },
    [batchDeleteSegmentsMutation]
  );

  // Pagination and filtering handlers
  const handleSegmentsSearch = useCallback((keyword: string) => {
    setSegmentsKeyword(keyword);
    setSegmentsPage(1);
  }, []);

  const handleSegmentsFilterChange = useCallback((filter: SegmentEnabledFilter) => {
    setSegmentsEnabledFilter(filter);
    setSegmentsPage(1);
  }, []);

  const handlePageChange = useCallback((page: number) => {
    setSegmentsPage(page);
  }, []);

  const handleSelectSegment = useCallback((segmentId: string) => {
    setSelectedSegments(prev => {
      if (prev.includes(segmentId)) {
        return prev.filter(id => id !== segmentId);
      } else {
        return [...prev, segmentId];
      }
    });
  }, []);

  const handleSelectAllSegments = useCallback(
    (checked: boolean) => {
      if (checked) {
        setSelectedSegments(segments.map(s => s.id));
      } else {
        setSelectedSegments([]);
      }
    },
    [segments]
  );

  // Loading state
  const isSegmentMutating =
    createSegmentMutation.isPending ||
    updateSegmentMutation.isPending ||
    batchEnableSegmentsMutation.isPending ||
    batchDisableSegmentsMutation.isPending ||
    batchDeleteSegmentsMutation.isPending;

  return {
    // Data
    segments,
    segmentsTotal,
    segmentsTotalPages,
    segmentsPageSize,
    isSegmentsLoading,
    isSegmentMutating,
    segmentsPage,
    segmentsKeyword,
    segmentsEnabledFilter,
    selectedSegments,

    // Actions
    createSegment,
    updateSegment,
    batchEnableSegments,
    batchDisableSegments,
    batchDeleteSegments,

    // Handlers
    handleSegmentsSearch,
    handleSegmentsFilterChange,
    handlePageChange,
    handleSelectSegment,
    handleSelectAllSegments,

    // Refetch
    refetchSegments,
  };
}
