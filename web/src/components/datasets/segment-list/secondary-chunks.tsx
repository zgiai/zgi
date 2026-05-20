'use client';

import React, { useState } from 'react';
import { useT } from '@/i18n';
import { ExternalLink, Pencil, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import type { SecondaryChunksProps } from './types';

/**
 * Secondary chunks component - inline preview with edit/delete actions
 */
export const SecondaryChunks = React.memo(function SecondaryChunks({
  segment,
  onViewAllChildSegments,
  onEditChildSegment,
  onDeleteChildSegment,
}: SecondaryChunksProps) {
  const t = useT('datasets');

  // Delete confirmation state
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deletingChildId, setDeletingChildId] = useState<string | null>(null);

  const handleDeleteClick = (childId: string) => {
    setDeletingChildId(childId);
    setDeleteConfirmOpen(true);
  };

  const handleConfirmDelete = () => {
    if (deletingChildId) {
      onDeleteChildSegment(segment.id, deletingChildId);
      setDeleteConfirmOpen(false);
      setDeletingChildId(null);
    }
  };

  return (
    <div>
      <div className="flex items-center gap-2">
        <Badge variant="subtle">
          {t('segments.secondaryChunk')}
          {segment?.child_chunks?.length ? ` (${segment.child_chunks.length})` : ''}
        </Badge>
        <Badge
          variant="subtle"
          className="cursor-pointer"
          onClick={() => onViewAllChildSegments(segment)}
        >
          <ExternalLink className="h-3 w-3 mr-1" />
          {segment?.child_chunks && segment.child_chunks.length > 0
            ? t('documents.viewAll')
            : t('segments.add')}
        </Badge>
      </div>

      {segment?.child_chunks && segment?.child_chunks?.length > 0 ? (
        <div className="flex flex-col gap-2 mt-3 max-h-64 overflow-y-auto">
          {segment.child_chunks?.map((child, index) => (
            <div
              key={child.id}
              className="group flex items-start gap-2 w-full bg-muted/50 px-3 py-1.5 rounded-md border border-border/50 hover:border-border"
            >
              <Badge variant="success" className="text-xs shrink-0">
                {`#S-${index + 1}`}
              </Badge>
              <span className="text-sm text-muted-foreground break-words whitespace-pre-wrap flex-1 min-w-0 line-clamp-2">
                {child.content}
              </span>
              <div className="flex items-center gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                <Button
                  variant="ghost"
                  size="xs"
                  className="h-6 w-6 p-0"
                  onClick={() => onEditChildSegment(segment.id, child.id, child.content)}
                >
                  <Pencil className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="xs"
                  className="h-6 w-6 p-0 text-destructive hover:text-destructive"
                  onClick={() => handleDeleteClick(child.id)}
                >
                  <Trash2 className="h-3 w-3" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-sm text-muted-foreground text-center my-2">{t('segments.noData')}</div>
      )}

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('segments.childSegmentDeleteConfirm')}
        confirmText={t('actions.delete')}
        cancelText={t('cancel')}
        onConfirm={handleConfirmDelete}
        variant="warning"
      />
    </div>
  );
});
