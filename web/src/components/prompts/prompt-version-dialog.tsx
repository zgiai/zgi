'use client';

import { useState } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type { PromptType, PromptVersionPayload } from '@/services/types/prompt';

interface PromptVersionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultType: PromptType;
  onSubmit: (payload: PromptVersionPayload) => Promise<unknown> | unknown;
}

export function PromptVersionDialog({
  open,
  onOpenChange,
  defaultType,
  onSubmit,
}: PromptVersionDialogProps) {
  const t = useT('prompts');
  const [content, setContent] = useState('');
  const [contentError, setContentError] = useState('');
  const [labels, setLabels] = useState('');
  const [commitMessage, setCommitMessage] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

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
        labels: labels
          .split(',')
          .map(item => item.trim())
          .filter(Boolean),
        commit_message: commitMessage.trim() || null,
      });
      setContent('');
      setContentError('');
      setLabels('');
      setCommitMessage('');
      onOpenChange(false);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('versions.createTitle')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label>{t('fields.content')}</Label>
            <Textarea
              value={content}
              onChange={e => {
                setContent(e.target.value);
                setContentError('');
              }}
              className="min-h-64 font-mono text-xs"
              placeholder={defaultType === 'chat' ? t('placeholders.chatContent') : t('placeholders.textContent')}
            />
            {contentError ? (
              <div className="text-sm text-destructive">{contentError}</div>
            ) : null}
          </div>
          <div className="space-y-2">
            <Label>{t('fields.labels')}</Label>
            <Input value={labels} onChange={e => setLabels(e.target.value)} placeholder={t('placeholders.labels')} />
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
            {t('actions.createVersion')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default PromptVersionDialog;
