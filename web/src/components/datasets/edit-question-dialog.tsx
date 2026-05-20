'use client';

import { useState } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';

interface EditQuestionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  question: string;
  onSubmit: (question: string) => void;
}

export function EditQuestionDialog({
  open,
  onOpenChange,
  question: initialQuestion,
  onSubmit,
}: EditQuestionDialogProps) {
  const t = useT('datasets');
  const [question, setQuestion] = useState(initialQuestion);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!question.trim()) return;

    setIsSubmitting(true);
    try {
      await onSubmit(question);
      onOpenChange(false);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px] p-0 overflow-hidden">
        <DialogHeader>
          <DialogTitle>{t('editQuestion.title')}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <DialogBody className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="question">{t('editQuestion.questionLabel')}</Label>
              <Textarea
                id="question"
                value={question}
                onChange={e => setQuestion(e.target.value)}
                placeholder={t('editQuestion.questionPlaceholder')}
                required
              />
            </div>
          </DialogBody>
          <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              {t('editQuestion.cancel')}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || !question.trim()}
              className="px-6 font-bold"
            >
              {isSubmitting ? t('editQuestion.saving') : t('editQuestion.saveButton')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
