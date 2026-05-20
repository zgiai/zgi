'use client';

import React, { useState, useEffect } from 'react';
import { useT } from '@/i18n';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';

interface ChildSegmentEditDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (content: string) => Promise<void>;
  initialContent: string;
  isLoading?: boolean;
}

/**
 * Dialog for editing a child segment's content
 */
export function ChildSegmentEditDialog({
  open,
  onClose,
  onSave,
  initialContent,
  isLoading = false,
}: ChildSegmentEditDialogProps) {
  const t = useT('datasets');
  const [content, setContent] = useState(initialContent);

  // Reset content when dialog opens with new initial content
  useEffect(() => {
    if (open) {
      setContent(initialContent);
    }
  }, [open, initialContent]);

  const handleSave = async () => {
    if (!content.trim()) return;
    await onSave(content);
  };

  return (
    <Dialog open={open} onOpenChange={isOpen => !isOpen && onClose()}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader className="flex flex-row items-center justify-between">
          <div className="flex items-center">
            <DialogTitle>{t('segments.childSegmentManagement')}</DialogTitle>
            <Badge variant="subtle" className="ml-2">
              {t('segments.childSegments')}
            </Badge>
            <span className="text-xs text-muted-foreground ml-2">
              {content.length} {t('statistics.words')}
            </span>
          </div>
        </DialogHeader>

        <DialogBody>
          <div className="mt-2">
            <Textarea
              placeholder={t('segments.childSegmentContent')}
              value={content}
              onChange={e => setContent(e.target.value)}
              className="min-h-[200px]"
            />

            <div className="flex justify-end gap-2 mt-4">
              <Button variant="outline" onClick={onClose}>
                {t('cancel')}
              </Button>
              <Button onClick={handleSave} disabled={!content.trim() || isLoading}>
                {isLoading ? t('saving') : t('save')}
              </Button>
            </div>
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
