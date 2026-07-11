'use client';

import { useEffect, useState, type ReactNode } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { PromptType, PromptVersionPayload } from '@/services/types/prompt';

interface PromptVersionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultType: PromptType;
  initialContent?: PromptVersionPayload['content'];
  initialCommitMessage?: string;
  title?: string;
  description?: string;
  submitLabel?: string;
  governanceNote?: ReactNode;
  onSubmit: (payload: PromptVersionPayload) => Promise<unknown> | unknown;
}

export function PromptVersionDialog({
  open,
  onOpenChange,
  defaultType,
  initialContent,
  initialCommitMessage,
  title,
  description,
  submitLabel,
  governanceNote,
  onSubmit,
}: PromptVersionDialogProps) {
  const t = useT('prompts');
  const [content, setContent] = useState('');
  const [contentError, setContentError] = useState('');
  const [commitMessage, setCommitMessage] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setContent(
      initialContent
        ? typeof initialContent === 'string'
          ? initialContent
          : JSON.stringify(initialContent, null, 2)
        : ''
    );
    setCommitMessage(initialCommitMessage ?? '');
    setContentError('');
  }, [initialCommitMessage, initialContent, open]);

  const handleSubmit = async () => {
    if (!content.trim()) return;
    setIsSubmitting(true);
    try {
      let parsedContent: PromptVersionPayload['content'] = content;
      if (defaultType !== 'text') {
        try {
          parsedContent = JSON.parse(content) as PromptVersionPayload['content'];
        } catch {
          setContentError(t('form.invalidChatJson'));
          return;
        }
      }
      setContentError('');
      await onSubmit({
        prompt_type: defaultType,
        content: parsedContent,
        labels: [],
        commit_message: commitMessage.trim() || null,
      });
      setContent('');
      setContentError('');
      setCommitMessage('');
      onOpenChange(false);
    } catch {
      // The caller is responsible for showing a localized error toast.
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title ?? t('versions.createTitle')}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        <DialogBody className="space-y-4">
          {governanceNote ? (
            <div className="rounded-lg border bg-muted/20 p-3 text-sm text-muted-foreground">
              {governanceNote}
            </div>
          ) : null}
          <div className="space-y-2">
            <Label>{t('fields.content')}</Label>
            <Textarea
              value={content}
              onChange={e => {
                setContent(e.target.value);
                setContentError('');
              }}
              className="min-h-64 font-mono text-xs"
              placeholder={
                defaultType === 'chat'
                  ? t('placeholders.chatContent')
                  : t('placeholders.textContent')
              }
            />
            {contentError ? <div className="text-sm text-destructive">{contentError}</div> : null}
          </div>
          <div className="space-y-2">
            <Label>{t('fields.commitMessage')}</Label>
            <Input
              value={commitMessage}
              onChange={e => setCommitMessage(e.target.value)}
              placeholder={t('placeholders.commitMessage')}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting || !content.trim()}>
            {submitLabel ?? t('actions.createVersion')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default PromptVersionDialog;
