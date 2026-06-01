'use client';

import React, { useState, useMemo } from 'react';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Plus,
  Search,
  Edit,
  Trash2,
  MoreHorizontal,
  ChevronDown,
  ChevronUp,
  X,
} from 'lucide-react';

import { useChildSegments } from '@/hooks/dataset/use-child-segments';
import type { ChildChunkDetail } from '@/services/types/dataset';

interface ChildSegmentsDialogProps {
  open: boolean;
  onClose: () => void;
  datasetId: string;
  documentId: string;
  segmentId: string;
  segmentPosition?: number;
}

export function ChildSegmentsDialog({
  open,
  onClose,
  datasetId,
  documentId,
  segmentId,
  segmentPosition: _segmentPosition,
}: ChildSegmentsDialogProps) {
  const t = useT('datasets');

  // Use dedicated hook for child segments management
  const {
    segment,
    childSegments,
    isLoading,
    isMutating: _isMutating,
    createChildSegment,
    updateChildSegment,
    deleteChildSegment,
  } = useChildSegments({
    datasetId,
    documentId,
    segmentId,
    enabled: open,
  });

  const [searchValue, setSearchValue] = useState('');
  const [expandedSegments, setExpandedSegments] = useState<Set<string>>(new Set());
  const [editingSegment, setEditingSegment] = useState<string | null>(null);
  const [editContent, setEditContent] = useState('');
  const [creatingNew, setCreatingNew] = useState(false);
  const [newContent, setNewContent] = useState('');
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deletingChildId, setDeletingChildId] = useState<string | null>(null);

  // Filter child segments based on search value (local filtering)
  const filteredChildSegments = useMemo(() => {
    if (!searchValue.trim()) return childSegments;
    const query = searchValue.toLowerCase().trim();
    return childSegments.filter(seg => seg.content.toLowerCase().includes(query));
  }, [childSegments, searchValue]);

  // Toggle segment expansion
  const toggleExpanded = (segmentId: string) => {
    setExpandedSegments(prev => {
      const newSet = new Set(prev);
      if (newSet.has(segmentId)) {
        newSet.delete(segmentId);
      } else {
        newSet.add(segmentId);
      }
      return newSet;
    });
  };

  // Start editing
  const startEdit = (childSegment: ChildChunkDetail) => {
    setEditingSegment(childSegment.id);
    setEditContent(childSegment.content);
  };

  // Cancel edit
  const cancelEdit = () => {
    setEditingSegment(null);
    setEditContent('');
  };

  // Save edit (toast is handled in the hook)
  const saveEdit = async () => {
    if (!editingSegment || !editContent.trim()) return;

    try {
      await updateChildSegment(editingSegment, editContent.trim());
      setEditingSegment(null);
      setEditContent('');
    } catch (_error) {
      // Error toast is handled in the hook
    }
  };

  // Start creating new
  const startCreateNew = () => {
    setCreatingNew(true);
    setNewContent('');
  };

  // Cancel create
  const cancelCreate = () => {
    setCreatingNew(false);
    setNewContent('');
  };

  // Save new (toast is handled in the hook)
  const saveNew = async () => {
    if (!newContent.trim()) return;

    try {
      await createChildSegment(newContent.trim());
      setCreatingNew(false);
      setNewContent('');
    } catch (_error) {
      // Error toast is handled in the hook
    }
  };

  // Open delete confirmation
  const handleOpenDelete = (childId: string) => {
    setDeletingChildId(childId);
    setDeleteConfirmOpen(true);
  };

  // Confirm delete child segment (toast is handled in the hook)
  const handleConfirmDelete = async () => {
    if (!deletingChildId) return;

    try {
      await deleteChildSegment(deletingChildId);
    } catch (_error) {
      // Error toast is handled in the hook
    } finally {
      setDeleteConfirmOpen(false);
      setDeletingChildId(null);
    }
  };

  // Format content for display
  const formatContent = (content: string, isExpanded: boolean) => {
    if (isExpanded) return content;
    return content.length > 100 ? `${content.substring(0, 100)}...` : content;
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-4xl max-h-[80vh] overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle>
            {t('segments.childSegmentManagementTitle', { position: segment?.position })}
          </DialogTitle>
          <DialogDescription>{t('segments.childSegmentDesc')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          {/* Parent segment info */}
          {segment && (
            <Card>
              <CardContent className="p-4">
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">
                      {t('segments.parentSegment')} #{segment.position}
                    </Badge>
                    <span className="text-sm text-muted-foreground">
                      {segment.word_count} {t('segments.wordCount')} · {segment.tokens}{' '}
                      {t('segmentEdit.tokens')}
                    </span>
                  </div>
                  <p className="text-sm text-muted-foreground line-clamp-3">{segment.content}</p>
                </div>
              </CardContent>
            </Card>
          )}

          {/* Controls */}
          <div className="flex items-center gap-3">
            <div className="flex-1 max-w-md relative mr-auto">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder={t('segments.childSegmentSearchPlaceholder')}
                value={searchValue}
                onChange={e => setSearchValue(e.target.value)}
                className="pl-9 pr-8"
              />
              {searchValue && (
                <button
                  type="button"
                  onClick={() => setSearchValue('')}
                  className="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-sm hover:bg-muted"
                >
                  <X className="h-3.5 w-3.5 text-muted-foreground" />
                </button>
              )}
            </div>

            <Button onClick={startCreateNew} disabled={creatingNew}>
              <Plus className="h-4 w-4" />
              {t('segments.childSegmentCreate')}
            </Button>
          </div>

          {/* Create new form */}
          {creatingNew && (
            <Card>
              <CardContent className="p-4">
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">{t('segments.new')}</Badge>
                    <span className="text-sm font-medium">{t('segments.childSegmentCreate')}</span>
                  </div>
                  <textarea
                    value={newContent}
                    onChange={e => setNewContent(e.target.value)}
                    placeholder={t('segments.childSegmentContentPlaceholder')}
                    className="w-full p-3 border rounded-md resize-none focus:outline-none focus:ring-2 focus:ring-primary"
                    rows={4}
                  />
                  <div className="flex items-center gap-2">
                    <Button size="sm" onClick={saveNew} disabled={!newContent.trim()}>
                      {t('save')}
                    </Button>
                    <Button size="sm" variant="outline" onClick={cancelCreate}>
                      {t('actions.cancel')}
                    </Button>
                  </div>
                </div>
              </CardContent>
            </Card>
          )}

          {/* Child segments list */}
          <div className="space-y-3">
            {isLoading && filteredChildSegments.length === 0 ? (
              // Loading skeleton
              Array.from({ length: 3 }).map((_, index) => (
                <Card key={index}>
                  <CardContent className="p-4">
                    <div className="space-y-3">
                      <div className="flex items-center gap-3">
                        <Skeleton className="h-4 w-16" />
                        <Skeleton className="h-4 w-20" />
                      </div>
                      <Skeleton className="h-16 w-full" />
                    </div>
                  </CardContent>
                </Card>
              ))
            ) : filteredChildSegments.length === 0 ? (
              // Empty state - differentiate between no data and no search results
              <Card>
                <CardContent className="p-8 text-center">
                  {searchValue.trim() ? (
                    // No search results
                    <>
                      <p className="text-muted-foreground">
                        {t('segments.childSegmentNoSearchResults')}
                      </p>
                      <Button
                        onClick={() => setSearchValue('')}
                        className="mt-4"
                        size="sm"
                        variant="outline"
                      >
                        {t('segments.clearSearch')}
                      </Button>
                    </>
                  ) : (
                    // No data at all
                    <>
                      <p className="text-muted-foreground">{t('segments.childSegmentNoData')}</p>
                      <Button onClick={startCreateNew} className="mt-4" size="sm">
                        <Plus className="h-4 w-4 mr-1" />
                        {t('segments.childSegmentCreateFirst')}
                      </Button>
                    </>
                  )}
                </CardContent>
              </Card>
            ) : (
              // Child segments list
              filteredChildSegments.map(childSegment => {
                const isExpanded = expandedSegments.has(childSegment.id);
                const isEditing = editingSegment === childSegment.id;

                return (
                  <Card key={childSegment.id}>
                    <CardContent className="p-4">
                      <div className="space-y-3">
                        {/* Header */}
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-3">
                            <Badge variant="outline">#{childSegment.position}</Badge>
                            <span className="text-sm text-muted-foreground">
                              {childSegment.word_count} {t('segments.wordCount')}
                            </span>
                          </div>

                          <div className="flex items-center gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => toggleExpanded(childSegment.id)}
                            >
                              {isExpanded ? (
                                <ChevronUp className="h-4 w-4" />
                              ) : (
                                <ChevronDown className="h-4 w-4" />
                              )}
                            </Button>
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button variant="ghost" size="sm">
                                  <MoreHorizontal className="h-4 w-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end">
                                <DropdownMenuItem onClick={() => startEdit(childSegment)}>
                                  <Edit className="h-4 w-4 mr-2" />
                                  {t('actions.edit')}
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem
                                  className="text-destructive"
                                  onClick={() => handleOpenDelete(childSegment.id)}
                                >
                                  <Trash2 className="h-4 w-4 mr-2" />
                                  {t('actions.delete')}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
                        </div>

                        {/* Content */}
                        {isEditing ? (
                          <div className="space-y-3">
                            <textarea
                              value={editContent}
                              onChange={e => setEditContent(e.target.value)}
                              className="w-full p-3 border rounded-md resize-none focus:outline-none focus:ring-2 focus:ring-primary"
                              rows={4}
                            />
                            <div className="flex items-center gap-2">
                              <Button size="sm" onClick={saveEdit} disabled={!editContent.trim()}>
                                {t('save')}
                              </Button>
                              <Button size="sm" variant="outline" onClick={cancelEdit}>
                                {t('actions.cancel')}
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <div>
                            <p className="text-sm whitespace-pre-wrap">
                              {formatContent(childSegment.content, isExpanded)}
                            </p>
                            {!isExpanded && childSegment.content.length > 100 && (
                              <Button
                                variant="link"
                                size="sm"
                                className="h-auto p-0 mt-1"
                                onClick={() => toggleExpanded(childSegment.id)}
                              >
                                {t('common.expandMore')}
                              </Button>
                            )}
                          </div>
                        )}

                        {/* Metadata */}
                        {isExpanded && (
                          <div className="pt-2 border-t">
                            <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                              <div>
                                {t('common.createdAt')}:{' '}
                                {new Date(childSegment.created_at * 1000).toLocaleString()}
                              </div>
                              <div>
                                {t('common.updatedAt')}:{' '}
                                {new Date(childSegment.updated_at * 1000).toLocaleString()}
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                );
              })
            )}

            {/* Load more - removed as hook handles all data */}
          </div>
        </DialogBody>

        <DialogFooter className="border-t pt-4">
          <div className="flex justify-end">
            <Button variant="outline" onClick={onClose}>
              {t('close')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('segments.delete')}
        description={t('segments.childSegmentDeleteConfirm')}
        confirmText={t('actions.delete')}
        cancelText={t('actions.cancel')}
        onConfirm={handleConfirmDelete}
        variant="danger"
      />
    </Dialog>
  );
}
