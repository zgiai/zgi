'use client';

import React, { useState, useCallback } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Plus, Filter, Eye, EyeOff, Trash2, Search, ChevronDown, ChevronUp } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Checkbox } from '@/components/ui/checkbox';
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Input } from '@/components/ui/input';

import { useDocumentSegments } from '@/hooks/dataset/use-document-segments';
import { SegmentEditDialog } from '@/components/datasets/segment-edit-dialog';
import { ChildSegmentsDialog } from '@/components/datasets/child-segments-dialog';
import { ChildSegmentEditDialog } from '@/components/datasets/child-segment-edit-dialog';
import { datasetService } from '@/services';
import { DATASET_KEYS } from '@/hooks/query-keys';

import { SegmentCard } from './segment-card';
import type {
  SegmentDetail,
  CreateSegmentRequest,
  UpdateSegmentRequest,
} from '@/services/types/dataset';

interface SegmentListProps {
  datasetId: string;
  documentId: string;
}

export function SegmentList({ datasetId, documentId }: SegmentListProps) {
  const t = useT('datasets');

  // Use document segments hook for segment CRUD and state management
  const {
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
    createSegment,
    updateSegment,
    batchEnableSegments,
    batchDisableSegments,
    batchDeleteSegments,
    handleSegmentsFilterChange,
    handlePageChange,
    handleSelectSegment,
    handleSelectAllSegments,
    handleSegmentsSearch,
  } = useDocumentSegments({
    datasetId,
    documentId,
  });

  // Local state
  const [searchValue, setSearchValue] = useState(segmentsKeyword);
  const [expandedSegments, setExpandedSegments] = useState<Set<string>>(new Set());

  // Dialog states
  const [isSegmentEditDialogOpen, setIsSegmentEditDialogOpen] = useState(false);
  const [editingSegment, setEditingSegment] = useState<SegmentDetail | undefined>();
  const [isChildSegmentsDialogOpen, setIsChildSegmentsDialogOpen] = useState(false);
  const [viewingChildSegmentsParent, setViewingChildSegmentsParent] = useState<
    SegmentDetail | undefined
  >();
  // Child segment edit dialog state
  const [isChildSegmentEditDialogOpen, setIsChildSegmentEditDialogOpen] = useState(false);
  const [editingChildSegment, setEditingChildSegment] = useState<{
    segmentId: string;
    childChunkId: string;
    content: string;
  } | null>(null);

  const queryClient = useQueryClient();

  // Child segment update mutation
  const updateChildSegmentMutation = useMutation({
    mutationFn: ({
      segmentId,
      childChunkId,
      content,
    }: {
      segmentId: string;
      childChunkId: string;
      content: string;
    }) =>
      datasetService.updateChildSegment(datasetId, documentId, segmentId, childChunkId, {
        content,
      }),
    onSuccess: () => {
      toast.success(t('segments.childSegmentUpdateSuccess'));
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segments(datasetId, documentId),
      });
      setIsChildSegmentEditDialogOpen(false);
      setEditingChildSegment(null);
    },
    onError: () => {
      toast.error(t('segments.childSegmentUpdateFailed'));
    },
  });

  // Child segment delete mutation
  const deleteChildSegmentMutation = useMutation({
    mutationFn: ({ segmentId, childChunkId }: { segmentId: string; childChunkId: string }) =>
      datasetService.deleteChildSegment(datasetId, documentId, segmentId, childChunkId),
    onSuccess: () => {
      toast.success(t('segments.childSegmentDeleteSuccess'));
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segments(datasetId, documentId),
      });
    },
    onError: () => {
      toast.error(t('segments.childSegmentDeleteFailed'));
    },
  });

  // Search handler
  const handleSearchSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      handleSegmentsSearch(searchValue);
    },
    [searchValue, handleSegmentsSearch]
  );

  // Toggle segment expansion
  const toggleExpanded = useCallback((segmentId: string) => {
    setExpandedSegments(prev => {
      const newSet = new Set(prev);
      if (newSet.has(segmentId)) {
        newSet.delete(segmentId);
      } else {
        newSet.add(segmentId);
      }
      return newSet;
    });
  }, []);

  // Toggle all segments expanded/collapsed
  const toggleAllExpanded = useCallback(() => {
    if (expandedSegments.size === segments.length) {
      // All expanded, collapse all
      setExpandedSegments(new Set());
    } else {
      // Expand all
      setExpandedSegments(new Set(segments.map(s => s.id)));
    }
  }, [expandedSegments.size, segments]);

  const allExpanded = expandedSegments.size === segments.length && segments.length > 0;

  // Segment handlers
  const handleCreateSegment = useCallback(() => {
    setEditingSegment(undefined);
    setIsSegmentEditDialogOpen(true);
  }, []);

  const handleEditSegment = useCallback((segment: SegmentDetail) => {
    setEditingSegment(segment);
    setIsSegmentEditDialogOpen(true);
  }, []);

  const handleSaveSegment = useCallback(
    async (data: CreateSegmentRequest | UpdateSegmentRequest) => {
      if (editingSegment) {
        await updateSegment(editingSegment.id, data as UpdateSegmentRequest);
      } else {
        await createSegment(data as CreateSegmentRequest);
      }
      setIsSegmentEditDialogOpen(false);
    },
    [editingSegment, updateSegment, createSegment]
  );

  // Open child segments dialog (unified handler for view/add)
  const handleViewChildChunks = useCallback((segment: SegmentDetail) => {
    setViewingChildSegmentsParent(segment);
    setIsChildSegmentsDialogOpen(true);
  }, []);

  // Child segment edit handler - opens edit dialog
  const handleEditChildSegment = useCallback(
    (segmentId: string, childChunkId: string, content: string) => {
      setEditingChildSegment({ segmentId, childChunkId, content });
      setIsChildSegmentEditDialogOpen(true);
    },
    []
  );

  // Child segment delete handler
  const handleDeleteChildSegment = useCallback(
    (segmentId: string, childChunkId: string) => {
      deleteChildSegmentMutation.mutate({ segmentId, childChunkId });
    },
    [deleteChildSegmentMutation]
  );

  // Child segment save handler
  const handleSaveChildSegment = useCallback(
    async (content: string) => {
      if (!editingChildSegment) return;
      await updateChildSegmentMutation.mutateAsync({
        segmentId: editingChildSegment.segmentId,
        childChunkId: editingChildSegment.childChunkId,
        content,
      });
    },
    [editingChildSegment, updateChildSegmentMutation]
  );

  // Batch actions
  const selectedCount = selectedSegments.length;
  const allSelected = segments.length > 0 && selectedSegments.length === segments.length;
  const someSelected = selectedSegments.length > 0;

  return (
    <div className="space-y-4">
      {/* Header and controls */}
      <div className="flex items-center justify-between">
        <form onSubmit={handleSearchSubmit} className="w-[200px]">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder={t('segments.searchPlaceholder')}
              value={searchValue}
              onChange={e => setSearchValue(e.target.value)}
              className="pl-9"
            />
          </div>
        </form>
        <div className="flex items-center gap-2">
          <Select
            value={segmentsEnabledFilter === 'all' ? 'all' : segmentsEnabledFilter.toString()}
            onValueChange={value => {
              if (value === 'all') {
                handleSegmentsFilterChange('all');
              } else {
                handleSegmentsFilterChange(value === 'true');
              }
            }}
          >
            <SelectTrigger className="w-32">
              <div className="flex items-center">
                <Filter className="h-4 w-4 mr-1" />
                <SelectValue />
              </div>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('search.all')}</SelectItem>
              <SelectItem value="true">{t('segments.enabled')}</SelectItem>
              <SelectItem value="false">{t('segments.disabled')}</SelectItem>
            </SelectContent>
          </Select>
          <Button
            onClick={toggleAllExpanded}
            variant="outline"
            size="lg"
            className="h-9"
            disabled={segments.length === 0}
          >
            {allExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            {allExpanded ? t('segments.collapseAll') : t('segments.expandAll')}
          </Button>
          <Button onClick={handleCreateSegment} size="lg" className="h-9">
            <Plus className="h-4 w-4" />
            {t('segments.create')}
          </Button>
        </div>
      </div>

      {/* Batch actions */}
      {someSelected && (
        <div className="flex justify-center gap-x-2 items-center absolute left-[50%] translate-x-[-50%] bottom-16 z-20 py-1 px-2 rounded-[10px] bg-background border border-secondary-foreground shadow-md">
          <span className="text-sm text-accent-foreground">
            {t('segments.selectCount', { count: selectedCount })}
          </span>
          <Separator orientation="vertical" className="h-4" />
          <Button variant="outline" size="sm" onClick={() => batchEnableSegments(selectedSegments)}>
            <Eye className="h-4 w-4 mr-1" />
            {t('actions.enable')}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => batchDisableSegments(selectedSegments)}
          >
            <EyeOff className="h-4 w-4 mr-1" />
            {t('actions.disable')}
          </Button>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => batchDeleteSegments(selectedSegments)}
          >
            <Trash2 className="h-4 w-4 mr-1" />
            {t('actions.delete')}
          </Button>
        </div>
      )}

      {/* Segments list */}
      <div className="space-y-3">
        {/* Select all header */}
        {segments.length > 0 && (
          <div className="flex items-center gap-2 px-3 py-2 rounded-lg">
            <Checkbox
              checked={allSelected}
              onCheckedChange={handleSelectAllSegments}
              aria-label={t('segments.selectAllLabel')}
            />
            <span className="text-sm text-muted-foreground">
              {t('segments.selectAll', { total: segments.length })}
            </span>
          </div>
        )}

        {/* Segments */}
        {isSegmentsLoading && segments.length === 0 ? (
          // Loading skeleton
          Array.from({ length: 3 }).map((_, index) => (
            <Card key={index}>
              <CardContent className="p-4">
                <div className="space-y-3">
                  <div className="flex items-center gap-3">
                    <Skeleton className="h-4 w-4" />
                    <Skeleton className="h-4 w-16" />
                    <Skeleton className="h-4 w-12" />
                    <Skeleton className="h-4 w-20" />
                  </div>
                  <Skeleton className="h-20 w-full" />
                </div>
              </CardContent>
            </Card>
          ))
        ) : segments.length === 0 ? (
          // Empty state
          <Card>
            <CardContent className="p-8 text-center">
              <p className="text-muted-foreground">{t('segments.noSegments')}</p>
              <Button onClick={handleCreateSegment} className="mt-4" size="sm">
                <Plus className="h-4 w-4 mr-1" />
                {t('segments.createFirst')}
              </Button>
            </CardContent>
          </Card>
        ) : (
          // Segments list
          segments.map((segment, index) => (
            <SegmentCard
              key={segment.id}
              segment={segment}
              displayIndex={(segmentsPage - 1) * segmentsPageSize + index + 1}
              isSelected={selectedSegments.includes(segment.id)}
              isExpanded={expandedSegments.has(segment.id)}
              onSelect={handleSelectSegment}
              onToggleExpand={toggleExpanded}
              onToggleEnabled={(segmentIds, enabled) => {
                if (enabled) {
                  batchEnableSegments(segmentIds);
                } else {
                  batchDisableSegments(segmentIds);
                }
              }}
              onEdit={handleEditSegment}
              onDelete={batchDeleteSegments}
              onViewChildChunks={handleViewChildChunks}
              onViewAllChildSegments={handleViewChildChunks}
              onEditChildSegment={handleEditChildSegment}
              onDeleteChildSegment={handleDeleteChildSegment}
            />
          ))
        )}

        {/* Pagination */}
        {segmentsTotalPages > 1 && (
          <Pagination
            currentPage={segmentsPage}
            totalPages={segmentsTotalPages}
            total={segmentsTotal}
            pageSize={segmentsPageSize}
            onPageChange={handlePageChange}
            className="mt-6"
          />
        )}
      </div>

      {/* Child Segment Edit Dialog */}
      <ChildSegmentEditDialog
        open={isChildSegmentEditDialogOpen}
        onClose={() => {
          setIsChildSegmentEditDialogOpen(false);
          setEditingChildSegment(null);
        }}
        initialContent={editingChildSegment?.content ?? ''}
        isLoading={updateChildSegmentMutation.isPending}
        onSave={handleSaveChildSegment}
      />

      {/* Segment Edit Dialog */}
      <SegmentEditDialog
        open={isSegmentEditDialogOpen}
        onClose={() => setIsSegmentEditDialogOpen(false)}
        segment={editingSegment}
        isLoading={isSegmentMutating}
        onSave={handleSaveSegment}
      />

      {/* Child Segments Dialog */}
      {viewingChildSegmentsParent && (
        <ChildSegmentsDialog
          open={isChildSegmentsDialogOpen}
          onClose={() => {
            setIsChildSegmentsDialogOpen(false);
            setViewingChildSegmentsParent(undefined);
          }}
          datasetId={datasetId}
          documentId={documentId}
          segmentId={viewingChildSegmentsParent.id}
          segmentPosition={viewingChildSegmentsParent.position}
        />
      )}
    </div>
  );
}
