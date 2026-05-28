'use client';

import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { restrictToVerticalAxis } from '@dnd-kit/modifiers';
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, Plus, Sparkles, Trash2 } from 'lucide-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { ChatOpeningGuideView } from '@/components/chat/ui/chat-opening-guide-view';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import { useT } from '@/i18n';
import {
  clampOpeningSlogan,
  OPENING_SLOGAN_MAX_LENGTH,
  type OpeningGuideEditorValue,
} from '@/utils/webapp/opening-statement';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import { cn } from '@/lib/utils';

const QUESTION_ID_PREFIX = 'opening-guide-question-';
let questionIdCounter = 0;

function createQuestionId(): string {
  questionIdCounter += 1;
  return `${QUESTION_ID_PREFIX}${Date.now()}-${questionIdCounter}`;
}

function normalizeQuestions(questions: string[] = []): string[] {
  return questions.filter(question => typeof question === 'string').slice(0, SUGGESTED_QUESTIONS_LIMIT);
}

function dedupeQuestions(questions: string[] = []): string[] {
  const seen = new Set<string>();
  return questions
    .map(question => question.trim())
    .filter(question => {
      if (!question) return false;
      const key = question.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
}

function mergeGeneratedQuestions(existing: string[], generated: string[]): string[] {
  const generatedQuestions = dedupeQuestions(generated);
  const generatedKeys = new Set(generatedQuestions.map(question => question.toLowerCase()));
  const remainingExisting = dedupeQuestions(existing).filter(
    question => !generatedKeys.has(question.toLowerCase())
  );

  return [...generatedQuestions, ...remainingExisting].slice(0, SUGGESTED_QUESTIONS_LIMIT);
}

interface OpeningGuideQuestionRowProps {
  id: string;
  index: number;
  question: string;
  placeholder: string;
  dragLabel: string;
  removeLabel: string;
  dragDisabled: boolean;
  onChange: (index: number, value: string) => void;
  onRemove: (index: number) => void;
}

const OpeningGuideQuestionRow: React.FC<OpeningGuideQuestionRowProps> = ({
  id,
  index,
  question,
  placeholder,
  dragLabel,
  removeLabel,
  dragDisabled,
  onChange,
  onRemove,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled: dragDisabled,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'flex w-full items-center gap-1 rounded-lg border border-border/60 bg-background px-1.5 py-1 shadow-sm transition-colors hover:border-border',
        isDragging && 'relative z-10 border-border bg-background opacity-90 shadow-md'
      )}
    >
      <Button
        type="button"
        variant="ghost"
        size="xs"
        disabled={dragDisabled}
        className={cn(
          'h-7 w-6 shrink-0 cursor-grab text-muted-foreground hover:text-foreground',
          isDragging && 'cursor-grabbing'
        )}
        aria-label={dragLabel}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5" />
      </Button>
      <Input
        value={question}
        containerClassName="min-w-0 flex-1"
        className="h-7 min-w-0 border-0 bg-transparent px-1.5 text-xs shadow-none focus-visible:ring-0"
        placeholder={placeholder}
        onChange={event => onChange(index, event.target.value)}
      />
      <Button
        type="button"
        variant="ghost"
        size="xs"
        className="h-7 w-7 shrink-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
        aria-label={removeLabel}
        onClick={() => onRemove(index)}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
};

export interface OpeningStatementDialogValue extends OpeningGuideEditorValue {
  suggestedQuestions: string[];
}

interface GenerateSuggestedQuestionsResult {
  questions: string[];
  warnings?: string[];
}

interface OpeningStatementDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: OpeningStatementDialogValue;
  onSave: (value: OpeningStatementDialogValue) => void;
  onGenerateSuggestedQuestions?: (
    value: OpeningStatementDialogValue
  ) => Promise<GenerateSuggestedQuestionsResult | undefined>;
  generatingSuggestedQuestions?: boolean;
  previewBrand?: OpeningGuideBrand;
}

/**
 * @component OpeningStatementDialog
 * @category Feature
 * @status Stable
 * @description Large editor dialog for workflow landing guides with live markdown preview.
 */
const OpeningStatementDialog: React.FC<OpeningStatementDialogProps> = ({
  open,
  onOpenChange,
  value,
  onSave,
  onGenerateSuggestedQuestions,
  generatingSuggestedQuestions,
  previewBrand,
}) => {
  const t = useT('agents');
  const tCommon = useT('common');
  const [draft, setDraft] = useState<OpeningStatementDialogValue>(value);
  const [questionIds, setQuestionIds] = useState<string[]>(() =>
    normalizeQuestions(value.suggestedQuestions).map(() => createQuestionId())
  );
  const [generatedWarnings, setGeneratedWarnings] = useState<string[]>([]);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const previewScrollRef = useRef<HTMLDivElement | null>(null);
  const syncingSourceRef = useRef<'editor' | 'preview' | null>(null);

  const questionSensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 4,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  const syncScrollPosition = useCallback(
    (
      source: HTMLTextAreaElement | HTMLDivElement | null,
      target: HTMLTextAreaElement | HTMLDivElement | null
    ) => {
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
      const nextValue = {
        ...value,
        suggestedQuestions: normalizeQuestions(value.suggestedQuestions),
      };
      setDraft(nextValue);
      setQuestionIds(nextValue.suggestedQuestions.map(() => createQuestionId()));
      setGeneratedWarnings([]);
    }
  }, [open, value]);

  useEffect(() => {
    const questionCount = normalizeQuestions(draft.suggestedQuestions).length;
    setQuestionIds(prev => {
      if (prev.length === questionCount) return prev;
      if (prev.length > questionCount) return prev.slice(0, questionCount);
      return [
        ...prev,
        ...Array.from({ length: questionCount - prev.length }, () => createQuestionId()),
      ];
    });
  }, [draft.suggestedQuestions]);

  useEffect(() => {
    if (!open) return;
    syncScrollPosition(textareaRef.current, previewScrollRef.current);
  }, [draft.message, open, syncScrollPosition]);

  const handleSave = () => {
    onSave({
      title: clampOpeningSlogan(draft.title),
      message: draft.message,
      suggestedQuestions: normalizeQuestions(draft.suggestedQuestions),
    });
    onOpenChange(false);
  };

  const addSuggestedQuestion = useCallback(() => {
    setDraft(prev => {
      const questions = normalizeQuestions(prev.suggestedQuestions);
      if (questions.length >= SUGGESTED_QUESTIONS_LIMIT) return prev;
      return {
        ...prev,
        suggestedQuestions: [...questions, ''],
      };
    });
  }, []);

  const updateSuggestedQuestion = useCallback((index: number, question: string) => {
    setDraft(prev => {
      const questions = normalizeQuestions(prev.suggestedQuestions);
      if (index < 0 || index >= questions.length) return prev;
      questions[index] = question;
      return {
        ...prev,
        suggestedQuestions: questions,
      };
    });
  }, []);

  const removeSuggestedQuestion = useCallback((index: number) => {
    setDraft(prev => {
      const questions = normalizeQuestions(prev.suggestedQuestions);
      if (index < 0 || index >= questions.length) return prev;
      return {
        ...prev,
        suggestedQuestions: questions.filter((_, questionIndex) => questionIndex !== index),
      };
    });
    setQuestionIds(prev => prev.filter((_, questionIndex) => questionIndex !== index));
  }, []);

  const handleQuestionDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;

      const activeIndex = questionIds.indexOf(String(active.id));
      const overIndex = questionIds.indexOf(String(over.id));
      if (activeIndex < 0 || overIndex < 0) return;

      setDraft(prev => {
        const questions = normalizeQuestions(prev.suggestedQuestions);
        if (
          activeIndex < 0 ||
          activeIndex >= questions.length ||
          overIndex < 0 ||
          overIndex >= questions.length
        ) {
          return prev;
        }
        return {
          ...prev,
          suggestedQuestions: arrayMove(questions, activeIndex, overIndex),
        };
      });
      setQuestionIds(prev => arrayMove(prev, activeIndex, overIndex));
    },
    [questionIds]
  );

  const handleGenerateSuggestedQuestions = useCallback(async () => {
    if (!onGenerateSuggestedQuestions) return;
    const result = await onGenerateSuggestedQuestions({
      ...draft,
      title: clampOpeningSlogan(draft.title),
      suggestedQuestions: normalizeQuestions(draft.suggestedQuestions),
    });
    if (!result?.questions.length) return;

    setDraft(prev => ({
      ...prev,
      suggestedQuestions: mergeGeneratedQuestions(prev.suggestedQuestions, result.questions),
    }));
    setGeneratedWarnings(result.warnings ?? []);
  }, [draft, onGenerateSuggestedQuestions]);

  const questions = normalizeQuestions(draft.suggestedQuestions);
  const visibleQuestionIds = questions.map(
    (_, index) => questionIds[index] ?? `${QUESTION_ID_PREFIX}pending-${index}`
  );
  const previewSuggestedQuestions = dedupeQuestions(questions);
  const configuredSuggestedQuestionCount = dedupeQuestions(questions).length;
  const canAddSuggestedQuestion = questions.length < SUGGESTED_QUESTIONS_LIMIT;
  const sloganCount = Array.from(draft.title).length;
  const hasPreviewContent =
    Boolean(draft.title.trim()) ||
    Boolean(draft.message.trim()) ||
    Boolean(previewBrand?.title?.trim()) ||
    previewSuggestedQuestions.length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="full"
        className="h-[calc(100vh-4rem)] w-[min(1800px,calc(100vw-2rem))] max-w-[min(1800px,calc(100vw-2rem))] overflow-hidden p-0"
      >
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('workflow.features.openingStatement.dialogTitle')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="min-h-0 overflow-hidden py-4">
          <div className="grid h-full min-h-0 gap-6 lg:grid-cols-2">
            <div className="flex h-full min-h-0 flex-col gap-4 pr-1">
              <div className="shrink-0 space-y-2">
                <div className="flex items-start justify-between gap-4">
                  <div className="space-y-1">
                    <Label className="text-sm font-semibold">
                      {t('workflow.features.openingStatement.sloganEditorLabel')}
                    </Label>
                    <p className="text-xs text-muted-foreground">
                      {t('workflow.features.openingStatement.sloganEditorDesc')}
                    </p>
                  </div>
                  <div className="shrink-0 text-xs text-muted-foreground">
                    {t('workflow.features.openingStatement.sloganCount', {
                      count: sloganCount,
                      max: OPENING_SLOGAN_MAX_LENGTH,
                    })}
                  </div>
                </div>
                <Input
                  value={draft.title}
                  placeholder={t('workflow.features.openingStatement.sloganPlaceholder')}
                  maxLength={OPENING_SLOGAN_MAX_LENGTH}
                  onChange={event =>
                    setDraft(prev => ({
                      ...prev,
                      title: clampOpeningSlogan(event.currentTarget.value),
                    }))
                  }
                />
              </div>

              <div className="flex min-h-0 flex-1 flex-col gap-2">
                <div className="shrink-0 space-y-1">
                  <Label className="text-sm font-semibold">
                    {t('workflow.features.openingStatement.messageEditorLabel')}
                  </Label>
                  <p className="text-xs text-muted-foreground">
                    {t('workflow.features.openingStatement.messageEditorDesc')}
                  </p>
                </div>
                <Textarea
                  ref={textareaRef}
                  value={draft.message}
                  placeholder={t('workflow.features.openingStatement.messagePlaceholder')}
                  className="h-full min-h-0 max-h-none flex-1 resize-none overflow-y-auto"
                  onChange={event =>
                    setDraft(prev => ({
                      ...prev,
                      message: event.currentTarget.value,
                    }))
                  }
                  onScroll={() => {
                    if (syncingSourceRef.current === 'preview') return;
                    syncingSourceRef.current = 'editor';
                    syncScrollPosition(textareaRef.current, previewScrollRef.current);
                    releaseSyncLock('editor');
                  }}
                />
              </div>

              <div className="shrink-0 space-y-2 rounded-lg border border-border/70 bg-muted/20 p-2.5">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <Label className="truncate text-sm font-semibold">
                      {t('workflow.features.suggestedQuestions.label')}
                    </Label>
                    <span className="shrink-0 rounded-md bg-background px-1.5 py-0.5 text-[11px] leading-4 text-muted-foreground ring-1 ring-border/60">
                      {t('workflow.features.suggestedQuestions.count', {
                        count: configuredSuggestedQuestionCount,
                        max: SUGGESTED_QUESTIONS_LIMIT,
                      })}
                    </span>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      className="bg-background"
                      loading={Boolean(generatingSuggestedQuestions)}
                      disabled={!onGenerateSuggestedQuestions}
                      onClick={handleGenerateSuggestedQuestions}
                    >
                      <Sparkles className="h-3.5 w-3.5" />
                      {t('workflow.features.suggestedQuestions.generate')}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      disabled={!canAddSuggestedQuestion}
                      onClick={addSuggestedQuestion}
                    >
                      <Plus className="h-3.5 w-3.5" />
                      {t('workflow.features.suggestedQuestions.add')}
                    </Button>
                  </div>
                </div>
                <p className="line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {t('workflow.features.suggestedQuestions.desc')}
                </p>
                {generatedWarnings.length > 0 ? (
                  <div className="rounded-md border border-amber-200 bg-amber-50 px-2.5 py-2 text-xs leading-5 text-amber-900">
                    {generatedWarnings.map((warning, index) => (
                      <div key={`${warning}-${index}`}>{warning}</div>
                    ))}
                  </div>
                ) : null}

                {questions.length === 0 ? (
                  <p className="rounded-md bg-muted/30 px-2.5 py-2 text-xs leading-5 text-muted-foreground">
                    {t('workflow.features.suggestedQuestions.empty')}
                  </p>
                ) : (
                  <DndContext
                    sensors={questionSensors}
                    collisionDetection={closestCenter}
                    modifiers={[restrictToVerticalAxis]}
                    onDragEnd={handleQuestionDragEnd}
                  >
                    <SortableContext
                      items={visibleQuestionIds}
                      strategy={verticalListSortingStrategy}
                    >
                      <div className="w-full space-y-1">
                        {questions.map((question, index) => (
                          <OpeningGuideQuestionRow
                            key={visibleQuestionIds[index]}
                            id={visibleQuestionIds[index]}
                            index={index}
                            question={question}
                            placeholder={t('workflow.features.suggestedQuestions.placeholder')}
                            dragLabel={t('workflow.features.suggestedQuestions.label')}
                            removeLabel={t('workflow.features.suggestedQuestions.remove')}
                            dragDisabled={questions.length <= 1}
                            onChange={updateSuggestedQuestion}
                            onRemove={removeSuggestedQuestion}
                          />
                        ))}
                      </div>
                    </SortableContext>
                  </DndContext>
                )}
              </div>
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
                  if (syncingSourceRef.current === 'editor') return;
                  syncingSourceRef.current = 'preview';
                  syncScrollPosition(previewScrollRef.current, textareaRef.current);
                  releaseSyncLock('preview');
                }}
              >
                {hasPreviewContent ? (
                  <div className="mx-auto flex min-h-full w-full min-w-0 max-w-6xl flex-col items-center justify-center overflow-hidden px-4 py-8">
                    <ChatOpeningGuideView
                      title={draft.title || previewBrand?.title}
                      message={draft.message}
                      iconType={previewBrand?.iconType}
                      icon={previewBrand?.icon}
                      iconBackground={previewBrand?.iconBackground}
                      iconSrc={previewBrand?.iconSrc}
                      suggestions={previewSuggestedQuestions}
                    />
                  </div>
                ) : (
                  <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                    {t('workflow.features.openingStatement.previewEmptyMessage')}
                  </div>
                )}
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
