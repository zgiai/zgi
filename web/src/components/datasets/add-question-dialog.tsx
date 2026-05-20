'use client';

import React, { useState, useEffect, useRef } from 'react';
import { useT } from '@/i18n';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';

import { type SegmentDetail } from '@/services/types/dataset';

interface AddQuestionDialogProps {
  open: boolean;
  onClose: () => void;
  segment?: SegmentDetail;
  isLoading: boolean;
  onSave: (segmentId: string, question: string) => Promise<void>;
}

export function AddQuestionDialog({
  open,
  onClose,
  segment,
  isLoading,
  onSave,
}: AddQuestionDialogProps) {
  const t = useT('datasets');

  // Form state
  const [question, setQuestion] = useState('');
  const [isContentExpanded, setIsContentExpanded] = useState(false);
  const [showExpandButton, setShowExpandButton] = useState(false);
  const contentRef = useRef<HTMLParagraphElement>(null);

  // Reset form when dialog opens/closes or segment changes
  useEffect(() => {
    if (open) {
      setQuestion('');
      setIsContentExpanded(false);
    }
  }, [open, segment]);

  // Check if content is truncated (exceeds 3 lines)
  useEffect(() => {
    if (!open || !segment || isContentExpanded) {
      setShowExpandButton(false);
      return;
    }

    // Simple check: when line-clamp-3 is applied, if scrollHeight > clientHeight, content is truncated
    const checkTruncation = () => {
      const element = contentRef.current;
      if (!element) return;
      setShowExpandButton(element.scrollHeight > element.clientHeight);
    };

    // Check after DOM is rendered
    const timer = setTimeout(checkTruncation, 100);
    return () => clearTimeout(timer);
  }, [open, segment, isContentExpanded, segment?.content]);

  // Validate form
  const isFormValid = question.trim().length > 0;

  // Handle save
  const handleSave = async () => {
    if (!isFormValid || !segment) {
      toast.error(t('addQuestion.validationFailed'), {
        description: t('addQuestion.enterValidQuestion'),
      });
      return;
    }

    onSave(segment.id, question.trim());
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
          <DialogTitle>{t('addQuestion.title')}</DialogTitle>
          <DialogDescription>
            {t('addQuestion.description', { position: segment?.position || '' })}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6 overflow-y-visible">
          {/* Segment content preview */}
          {segment && (
            <div className="p-4 bg-muted rounded-lg space-y-2">
              <h4 className="text-sm font-medium">{t('addQuestion.segmentContent')}</h4>
              <div className="relative">
                <p
                  ref={contentRef}
                  className={cn(
                    'text-sm text-muted-foreground',
                    !isContentExpanded && 'line-clamp-3'
                  )}
                >
                  {segment.content}
                </p>
                {showExpandButton && !isContentExpanded && (
                  <button
                    type="button"
                    onClick={e => {
                      e.stopPropagation();
                      setIsContentExpanded(true);
                    }}
                    className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors mt-1 absolute bottom-0 right-[-2px]"
                  >
                    <ChevronDown className="h-4 w-4" />
                  </button>
                )}
                {showExpandButton && isContentExpanded && (
                  <button
                    type="button"
                    onClick={e => {
                      e.stopPropagation();
                      setIsContentExpanded(false);
                    }}
                    className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors mt-1 absolute bottom-0 right-[-2px]"
                  >
                    <ChevronUp className="h-6 w-6" />
                  </button>
                )}
              </div>
            </div>
          )}

          {/* Question input */}
          <div className="space-y-2">
            <Label htmlFor="question">
              {t('addQuestion.questionContent')} <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="question"
              value={question}
              onChange={e => setQuestion(e.target.value)}
              placeholder={t('addQuestion.questionPlaceholder')}
              rows={4}
              className="resize-none"
            />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>{t('addQuestion.characterCount', { count: question.length })}</span>
              <span>{t('addQuestion.suggestedLength')}</span>
            </div>
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isLoading}>
            {t('addQuestion.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={!isFormValid || isLoading || !segment}>
            {isLoading ? t('addQuestion.adding') : t('addQuestion.addButton')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
