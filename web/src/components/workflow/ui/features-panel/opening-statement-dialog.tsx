'use client';

import React, { useCallback, useEffect, useRef, useState } from 'react';
import { Dialog, DialogBody, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { ChatOpeningMessage } from '@/components/chat/ui/chat-opening-message';
import { ChatHomeView } from '@/components/chat/ui/chat-home-view';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import {
  clampOpeningSlogan,
  OPENING_SLOGAN_MAX_LENGTH,
  type OpeningStatementType,
} from '@/utils/webapp/opening-statement';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';

interface OpeningStatementDialogValue {
  type: OpeningStatementType;
  slogan: string;
  message: string;
}

interface OpeningStatementDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: OpeningStatementDialogValue;
  onSave: (value: OpeningStatementDialogValue) => void;
  previewBrand?: OpeningGuideBrand;
  suggestedQuestions?: string[];
}

/**
 * @component OpeningStatementDialog
 * @category Feature
 * @status Stable
 * @description Large editor dialog for workflow opening statements with live markdown preview.
 * @usage Open from the workflow features panel to edit the landing-page opening statement.
 * @example
 * <OpeningStatementDialog open={open} onOpenChange={setOpen} value={value} onSave={handleSave} />
 */
const OpeningStatementDialog: React.FC<OpeningStatementDialogProps> = ({
  open,
  onOpenChange,
  value,
  onSave,
  previewBrand,
  suggestedQuestions,
}) => {
  const t = useT('agents');
  const tCommon = useT('common');
  const [draft, setDraft] = useState<OpeningStatementDialogValue>(value);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const previewScrollRef = useRef<HTMLDivElement | null>(null);
  const syncingSourceRef = useRef<'editor' | 'preview' | null>(null);

  const syncScrollPosition = useCallback(
    (source: HTMLTextAreaElement | HTMLDivElement | null, target: HTMLTextAreaElement | HTMLDivElement | null) => {
      if (!source || !target) return;

      const sourceScrollable = source.scrollHeight - source.clientHeight;
      const targetScrollable = target.scrollHeight - target.clientHeight;

      if (sourceScrollable <= 0 || targetScrollable <= 0) {
        target.scrollTop = 0;
        return;
      }

      const ratio = source.scrollTop / sourceScrollable;
      target.scrollTop = ratio * targetScrollable;
    },
    []
  );

  const releaseSyncLock = useCallback((owner: 'editor' | 'preview') => {
    window.requestAnimationFrame(() => {
      if (syncingSourceRef.current === owner) {
        syncingSourceRef.current = null;
      }
    });
  }, []);

  useEffect(() => {
    if (open) {
      setDraft(value);
    }
  }, [open, value]);

  useEffect(() => {
    if (!open || draft.type !== 'message') return;
    syncScrollPosition(textareaRef.current, previewScrollRef.current);
  }, [draft, open, syncScrollPosition]);

  const handleSave = () => {
    onSave(draft);
    onOpenChange(false);
  };

  const activeValue = draft.type === 'slogan' ? draft.slogan : draft.message;
  const activePlaceholder =
    draft.type === 'slogan'
      ? t('workflow.features.openingStatement.sloganPlaceholder')
      : t('workflow.features.openingStatement.messagePlaceholder');
  const editorLabel =
    draft.type === 'slogan'
      ? t('workflow.features.openingStatement.sloganEditorLabel')
      : t('workflow.features.openingStatement.messageEditorLabel');
  const editorDesc =
    draft.type === 'slogan'
      ? t('workflow.features.openingStatement.sloganEditorDesc')
      : t('workflow.features.openingStatement.messageEditorDesc');
  const previewEmpty =
    draft.type === 'slogan'
      ? t('workflow.features.openingStatement.previewEmptySlogan')
      : t('workflow.features.openingStatement.previewEmptyMessage');
  const sloganCount = Array.from(draft.slogan).length;
  const previewSuggestedQuestions = (suggestedQuestions ?? [])
    .map(question => question.trim())
    .filter(Boolean)
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="full"
        className="h-[calc(100vh-4rem)] w-[min(1800px,calc(100vw-2rem))] max-w-[min(1800px,calc(100vw-2rem))] p-0 overflow-hidden"
      >
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('workflow.features.openingStatement.dialogTitle')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="min-h-0 overflow-hidden py-4">
          <div className="flex h-full min-h-0 flex-col gap-4">
            <div className="space-y-3">
              <div className="space-y-1">
                <Label className="text-sm font-semibold">
                  {t('workflow.features.openingStatement.typeLabel')}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t('workflow.features.openingStatement.typeDesc')}
                </p>
              </div>
              <Tabs
                value={draft.type}
                onValueChange={nextValue =>
                  setDraft(prev => ({
                    ...prev,
                    type: nextValue === 'message' ? 'message' : 'slogan',
                  }))
                }
              >
                <TabsList className="grid w-full max-w-md grid-cols-2">
                  <TabsTrigger value="slogan">
                    {t('workflow.features.openingStatement.types.slogan')}
                  </TabsTrigger>
                  <TabsTrigger value="message">
                    {t('workflow.features.openingStatement.types.message')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </div>

            <div className="grid h-full min-h-0 gap-6 lg:grid-cols-2">
            <div className="flex min-h-0 flex-col gap-3">
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-1">
                  <Label className="text-sm font-semibold">{editorLabel}</Label>
                  <p className="text-xs text-muted-foreground">{editorDesc}</p>
                </div>
                {draft.type === 'slogan' ? (
                  <div className="shrink-0 text-xs text-muted-foreground">
                    {t('workflow.features.openingStatement.sloganCount', {
                      count: sloganCount,
                      max: OPENING_SLOGAN_MAX_LENGTH,
                    })}
                  </div>
                ) : null}
              </div>
              <Textarea
                ref={textareaRef}
                value={activeValue}
                placeholder={activePlaceholder}
                className="h-full min-h-0 max-h-none flex-1 resize-none overflow-y-auto"
                onChange={event => {
                  const nextValue = event.currentTarget.value;
                  setDraft(prev =>
                    prev.type === 'slogan'
                      ? {
                          ...prev,
                          slogan: clampOpeningSlogan(nextValue),
                        }
                      : {
                          ...prev,
                          message: nextValue,
                        }
                  );
                }}
                maxLength={
                  draft.type === 'slogan' ? OPENING_SLOGAN_MAX_LENGTH : undefined
                }
                onScroll={() => {
                  if (draft.type !== 'message') return;
                  if (syncingSourceRef.current === 'preview') return;
                  syncingSourceRef.current = 'editor';
                  syncScrollPosition(textareaRef.current, previewScrollRef.current);
                  releaseSyncLock('editor');
                }}
              />
            </div>

            <div className="flex min-h-0 flex-col gap-3">
              <div className="space-y-1">
                <Label className="text-sm font-semibold">
                  {t('workflow.features.openingStatement.previewLabel')}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t('workflow.features.openingStatement.previewDesc')}
                </p>
              </div>
              <div
                ref={previewScrollRef}
                className="min-h-0 flex-1 overflow-y-auto rounded-lg border bg-background p-3"
                onScroll={() => {
                  if (draft.type !== 'message') return;
                  if (syncingSourceRef.current === 'editor') return;
                  syncingSourceRef.current = 'preview';
                  syncScrollPosition(previewScrollRef.current, textareaRef.current);
                  releaseSyncLock('preview');
                }}
              >
                {activeValue.trim().length > 0 ? (
                  draft.type === 'slogan' ? (
                    <div className="mx-auto flex min-h-full w-full min-w-0 max-w-6xl overflow-hidden">
                      <ChatHomeView
                        className="max-w-none"
                        title={draft.slogan}
                        suggestions={previewSuggestedQuestions}
                      />
                    </div>
                  ) : (
                    <div className="mx-auto flex min-h-full w-full min-w-0 max-w-6xl flex-col items-center justify-center overflow-hidden px-4 py-8">
                      <ChatOpeningMessage
                        content={draft.message}
                        title={previewBrand?.title}
                        iconType={previewBrand?.iconType}
                        icon={previewBrand?.icon}
                        iconBackground={previewBrand?.iconBackground}
                        iconSrc={previewBrand?.iconSrc}
                        suggestions={previewSuggestedQuestions}
                      />
                    </div>
                  )
                ) : (
                  <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                    {previewEmpty}
                  </div>
                )}
              </div>
            </div>
          </div>
          </div>
        </DialogBody>

        <DialogFooter className="border-t bg-neutral-50/50 px-6 pb-6 pt-4">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {tCommon('close')}
          </Button>
          <Button type="button" onClick={handleSave}>
            {tCommon('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default OpeningStatementDialog;
