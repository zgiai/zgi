'use client';

import React, { useState, useEffect } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { toast } from 'sonner';

import {
  type SegmentDetail,
  type CreateSegmentRequest,
  type UpdateSegmentRequest,
} from '@/services/types/dataset';

interface SegmentEditDialogProps {
  open: boolean;
  onClose: () => void;
  segment?: SegmentDetail; // undefined for create mode
  isLoading: boolean;
  onSave: (data: CreateSegmentRequest | UpdateSegmentRequest) => Promise<void>;
}

export function SegmentEditDialog({
  open,
  onClose,
  segment,
  isLoading,
  onSave,
}: SegmentEditDialogProps) {
  const t = useT('datasets');

  const isEditMode = !!segment;
  const isHierarchicalMode = true; // All modes are treated as hierarchical now

  // Form state
  const [content, setContent] = useState('');
  const [keywords, setKeywords] = useState<string[]>([]);
  const [regenerateChildChunks, setRegenerateChildChunks] = useState(false);

  // Reset form when dialog opens/closes or segment changes
  useEffect(() => {
    if (open) {
      if (segment) {
        setContent(segment.content || '');
        setKeywords(segment.keywords || []);
        setRegenerateChildChunks(false);
      } else {
        setContent('');
        setKeywords([]);
        setRegenerateChildChunks(false);
      }
    }
  }, [open, segment]);

  // Validate form
  const isFormValid = content.trim().length > 0;

  // Handle save
  const handleSave = async () => {
    if (!isFormValid) {
      toast.error(t('segmentEdit.validationFailed'));
      return;
    }

    const data: CreateSegmentRequest | UpdateSegmentRequest = {
      content: content.trim(),
      ...(keywords.length > 0 && { keywords }),
      ...(isHierarchicalMode && isEditMode && { regenerate_child_chunks: regenerateChildChunks }),
    };

    try {
      await onSave(data);
      onClose();
      // Toast is handled in the hook (use-document-detail.ts / use-document-segments.ts)
    } catch {
      // Error toast is handled in the hook
    }
  };

  // Handle dialog close
  const handleClose = () => {
    if (!isLoading) {
      onClose();
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEditMode
              ? t('segmentEdit.editTitle', { position: segment?.position })
              : t('segmentEdit.createTitle')}
          </DialogTitle>
          <DialogDescription>
            {isEditMode ? t('segmentEdit.editDescription') : t('segmentEdit.createDescription')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          {/* Content */}
          <div className="space-y-2">
            <Label htmlFor="content">
              {t('segmentEdit.contentLabel')} <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="content"
              value={content}
              onChange={e => setContent(e.target.value)}
              placeholder={t('segmentEdit.contentPlaceholder')}
              rows={8}
              className="resize-none"
            />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>{t('segmentEdit.characterCount', { count: content.length })}</span>
              <span>{t('segmentEdit.suggestedLength')}</span>
            </div>
          </div>

          {/* Keywords - hidden for now */}

          {/* Hierarchical Mode Options */}
          {isHierarchicalMode && isEditMode && (
            <div className="space-y-2">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="regenerate-child-chunks"
                  checked={regenerateChildChunks}
                  onCheckedChange={checked => setRegenerateChildChunks(!!checked)}
                />
                <Label htmlFor="regenerate-child-chunks">
                  {t('segmentEdit.regenerateChildChunks')}
                </Label>
              </div>
              <div className="text-xs text-muted-foreground">
                {t('segmentEdit.regenerateChildChunksHint')}
              </div>
            </div>
          )}

          {/* Current segment info - hidden for now */}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isLoading}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={!isFormValid || isLoading}>
            {isLoading
              ? t('segmentEdit.saving')
              : isEditMode
                ? t('segmentEdit.updateButton')
                : t('segmentEdit.createButton')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
