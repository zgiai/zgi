'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import { DATASET_KEYS } from '@/hooks/query-keys';
import type {
  SegmentDetail,
  ChildChunkDetail,
  CreateChildSegmentRequest,
  UpdateChildSegmentRequest,
} from '@/services/types/dataset';

interface UseChildSegmentsProps {
  datasetId: string;
  documentId: string;
  segmentId: string;
  /** Whether to enable queries */
  enabled?: boolean;
}

interface UseChildSegmentsReturn {
  /** Parent segment data */
  segment: SegmentDetail | undefined;
  /** Child segments list */
  childSegments: ChildChunkDetail[];
  /** Loading state */
  isLoading: boolean;
  /** Mutation pending state */
  isMutating: boolean;
  /** Create child segment */
  createChildSegment: (content: string) => Promise<void>;
  /** Update child segment */
  updateChildSegment: (childChunkId: string, content: string) => Promise<void>;
  /** Delete child segment */
  deleteChildSegment: (childChunkId: string) => Promise<void>;
  /** Refetch segment data */
  refetch: () => void;
}

/**
 * Hook for managing child segments of a specific segment
 * Encapsulates all child segment CRUD operations with automatic data sync
 */
export function useChildSegments({
  datasetId,
  documentId,
  segmentId,
  enabled = true,
}: UseChildSegmentsProps): UseChildSegmentsReturn {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  const queryEnabled = enabled && !!datasetId && !!documentId && !!segmentId;

  // Query params for segments
  const queryParams = { page: 1, limit: 100, keyword: '', enabled: 'all' as const };

  // Query for segment data (includes child_chunks)
  const {
    data: segmentsResponse,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: DATASET_KEYS.segmentList(datasetId, documentId, queryParams),
    queryFn: () => datasetService.getSegments(datasetId, documentId, queryParams),
    enabled: queryEnabled,
    staleTime: 30 * 1000,
  });

  // Find the specific segment from the list
  const segment = segmentsResponse?.data?.data?.find((s: SegmentDetail) => s.id === segmentId);
  const childSegments = segment?.child_chunks ?? [];

  // Create child segment mutation
  const createMutation = useMutation({
    mutationFn: (data: CreateChildSegmentRequest) =>
      datasetService.createChildSegment(datasetId, documentId, segmentId, data),
    onSuccess: () => {
      toast.success(t('segments.childSegmentCreateSuccess'));
      // Invalidate segments query to refetch
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segments(datasetId, documentId),
      });
    },
    onError: () => {
      toast.error(t('segments.childSegmentCreateFailed'));
    },
  });

  // Update child segment mutation
  const updateMutation = useMutation({
    mutationFn: ({
      childChunkId,
      data,
    }: {
      childChunkId: string;
      data: UpdateChildSegmentRequest;
    }) => datasetService.updateChildSegment(datasetId, documentId, segmentId, childChunkId, data),
    onSuccess: () => {
      toast.success(t('segments.childSegmentUpdateSuccess'));
      // Invalidate segments query to refetch
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segments(datasetId, documentId),
      });
    },
    onError: () => {
      toast.error(t('segments.childSegmentUpdateFailed'));
    },
  });

  // Delete child segment mutation
  const deleteMutation = useMutation({
    mutationFn: (childChunkId: string) =>
      datasetService.deleteChildSegment(datasetId, documentId, segmentId, childChunkId),
    onSuccess: () => {
      toast.success(t('segments.childSegmentDeleteSuccess'));
      // Invalidate segments query to refetch
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segments(datasetId, documentId),
      });
    },
    onError: () => {
      toast.error(t('segments.childSegmentDeleteFailed'));
    },
  });

  // Wrapped mutation functions
  const createChildSegment = async (content: string) => {
    await createMutation.mutateAsync({ content });
  };

  const updateChildSegment = async (childChunkId: string, content: string) => {
    await updateMutation.mutateAsync({ childChunkId, data: { content } });
  };

  const deleteChildSegment = async (childChunkId: string) => {
    await deleteMutation.mutateAsync(childChunkId);
  };

  const isMutating =
    createMutation.isPending || updateMutation.isPending || deleteMutation.isPending;

  return {
    segment,
    childSegments,
    isLoading,
    isMutating,
    createChildSegment,
    updateChildSegment,
    deleteChildSegment,
    refetch,
  };
}
