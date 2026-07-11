'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
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
import { GripVertical, Loader2, Plus, Sparkles, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { AGENT_HOME_TITLE_MAX_LENGTH, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH } from '../constants';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

const QUESTION_ID_PREFIX = 'agent-opening-question-';
let questionIdCounter = 0;

function createQuestionId(): string {
  questionIdCounter += 1;
  return `${QUESTION_ID_PREFIX}${Date.now()}-${questionIdCounter}`;
}

function normalizeQuestions(questions: string[] = []): string[] {
  return questions
    .filter(question => typeof question === 'string')
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
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

interface AgentQuestionRowProps {
  id: string;
  index: number;
  question: string;
  placeholder: string;
  dragLabel: string;
  removeLabel: string;
  dragDisabled: boolean;
  readOnly: boolean;
  onChange: (index: number, value: string) => void;
  onRemove: (index: number) => void;
}

function AgentQuestionRow({
  id,
  index,
  question,
  placeholder,
  dragLabel,
  removeLabel,
  dragDisabled,
  readOnly,
  onChange,
  onRemove,
}: AgentQuestionRowProps) {
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
        disabled={readOnly}
        onChange={event => onChange(index, event.target.value)}
      />
      <Button
        type="button"
        variant="ghost"
        size="xs"
        disabled={readOnly}
        className="h-7 w-7 shrink-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
        aria-label={removeLabel}
        onClick={() => onRemove(index)}
      >
        <Trash2 className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

interface AgentSuggestedQuestionsEditorProps {
  questions: string[];
  isGeneratingSuggestions: boolean;
  readOnly?: boolean;
  onGenerateSuggestedQuestions: () => void;
  onChangeSuggestedQuestions: (value: string[]) => void;
}

function AgentSuggestedQuestionsEditor({
  questions,
  isGeneratingSuggestions,
  readOnly = false,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
}: AgentSuggestedQuestionsEditorProps) {
  const t = useT('agents.agentRuntime');
  const normalizedQuestions = useMemo(() => normalizeQuestions(questions), [questions]);
  const previewQuestions = useMemo(
    () => dedupeQuestions(normalizedQuestions),
    [normalizedQuestions]
  );
  const [questionIds, setQuestionIds] = useState<string[]>(() =>
    normalizedQuestions.map(() => createQuestionId())
  );
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

  useEffect(() => {
    setQuestionIds(prev => {
      if (prev.length === normalizedQuestions.length) return prev;
      if (prev.length > normalizedQuestions.length) {
        return prev.slice(0, normalizedQuestions.length);
      }
      return [
        ...prev,
        ...Array.from({ length: normalizedQuestions.length - prev.length }, () =>
          createQuestionId()
        ),
      ];
    });
  }, [normalizedQuestions.length]);

  const updateQuestion = useCallback(
    (index: number, question: string) => {
      onChangeSuggestedQuestions(
        normalizedQuestions.map((item, itemIndex) => (itemIndex === index ? question : item))
      );
    },
    [normalizedQuestions, onChangeSuggestedQuestions]
  );

  const addQuestion = useCallback(() => {
    if (normalizedQuestions.length >= SUGGESTED_QUESTIONS_LIMIT) return;
    onChangeSuggestedQuestions([...normalizedQuestions, '']);
  }, [normalizedQuestions, onChangeSuggestedQuestions]);

  const removeQuestion = useCallback(
    (index: number) => {
      onChangeSuggestedQuestions(
        normalizedQuestions.filter((_, questionIndex) => questionIndex !== index)
      );
      setQuestionIds(prev => prev.filter((_, questionIndex) => questionIndex !== index));
    },
    [normalizedQuestions, onChangeSuggestedQuestions]
  );

  const handleQuestionDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;

      const activeIndex = questionIds.indexOf(String(active.id));
      const overIndex = questionIds.indexOf(String(over.id));
      if (activeIndex < 0 || overIndex < 0) return;

      onChangeSuggestedQuestions(arrayMove(normalizedQuestions, activeIndex, overIndex));
      setQuestionIds(prev => arrayMove(prev, activeIndex, overIndex));
    },
    [normalizedQuestions, onChangeSuggestedQuestions, questionIds]
  );

  const visibleQuestionIds = normalizedQuestions.map(
    (_, index) => questionIds[index] ?? `${QUESTION_ID_PREFIX}pending-${index}`
  );
  const canGenerateSuggestions = !readOnly && !isGeneratingSuggestions;
  const canAddQuestion = !readOnly && normalizedQuestions.length < SUGGESTED_QUESTIONS_LIMIT;

  return (
    <div className="rounded-lg border border-border/70 bg-background p-2.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="rounded-md bg-muted px-1.5 py-0.5 text-[11px] leading-4 text-muted-foreground ring-1 ring-border/60">
          {t('suggestions.count', {
            count: previewQuestions.length,
            max: SUGGESTED_QUESTIONS_LIMIT,
          })}
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            variant="outline"
            size="xs"
            className="h-7 gap-1.5 bg-background px-2 text-xs"
            disabled={!canGenerateSuggestions}
            onClick={onGenerateSuggestedQuestions}
          >
            {isGeneratingSuggestions ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Sparkles className="h-3.5 w-3.5" />
            )}
            {t('suggestions.generate')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="h-7 gap-1.5 px-2 text-xs"
            disabled={!canAddQuestion}
            onClick={addQuestion}
          >
            <Plus className="h-3.5 w-3.5" />
            {t('suggestions.add')}
          </Button>
        </div>
      </div>

      {normalizedQuestions.length === 0 ? (
        <p className="mt-2 rounded-md bg-muted/30 px-2.5 py-2 text-xs leading-5 text-muted-foreground">
          {t('suggestions.empty')}
        </p>
      ) : (
        <DndContext
          sensors={questionSensors}
          collisionDetection={closestCenter}
          modifiers={[restrictToVerticalAxis]}
          onDragEnd={handleQuestionDragEnd}
        >
          <SortableContext items={visibleQuestionIds} strategy={verticalListSortingStrategy}>
            <div className="mt-2 w-full space-y-1.5">
              {normalizedQuestions.map((question, index) => (
                <AgentQuestionRow
                  key={visibleQuestionIds[index]}
                  id={visibleQuestionIds[index]}
                  index={index}
                  question={question}
                  placeholder={t('suggestions.placeholder')}
                  dragLabel={t('suggestions.drag')}
                  removeLabel={t('suggestions.delete')}
                  dragDisabled={readOnly || normalizedQuestions.length <= 1}
                  readOnly={readOnly}
                  onChange={updateQuestion}
                  onRemove={removeQuestion}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}
    </div>
  );
}

interface AgentRuntimeExperienceSectionProps {
  open: boolean;
  homeTitle: string;
  inputPlaceholder: string;
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onGenerateSuggestedQuestions: () => void;
  onChangeSuggestedQuestions: (value: string[]) => void;
}

export function AgentRuntimeExperienceSection({
  open,
  homeTitle,
  inputPlaceholder,
  suggestedQuestions,
  isGeneratingSuggestions,
  defaultHomeTitle,
  defaultInputPlaceholder,
  readOnly = false,
  onToggleSection,
  onChangeHomeTitle,
  onChangeInputPlaceholder,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
}: AgentRuntimeExperienceSectionProps) {
  const t = useT('agents.agentRuntime');
  const normalizedSuggestedQuestions = normalizeQuestions(suggestedQuestions);

  return (
    <RuntimeSection
      title={t('sections.experience')}
      section="experience"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="space-y-3">
        <div className="text-xs font-semibold text-muted-foreground">
          {t('experience.homeGroup')}
        </div>
        <div className="space-y-1.5">
          <div className="text-xs font-medium text-muted-foreground">
            {t('appearance.homeTitle')}
          </div>
          <Input
            value={homeTitle}
            maxLength={AGENT_HOME_TITLE_MAX_LENGTH}
            showCharacterCount
            placeholder={defaultHomeTitle}
            disabled={readOnly}
            onChange={event =>
              onChangeHomeTitle(
                Array.from(event.target.value).slice(0, AGENT_HOME_TITLE_MAX_LENGTH).join('')
              )
            }
          />
        </div>
      </div>

      <div className="space-y-3 pt-2">
        <div className="text-xs font-semibold text-muted-foreground">
          {t('experience.inputGroup')}
        </div>
        <div className="space-y-1.5">
          <div className="text-xs font-medium text-muted-foreground">
            {t('appearance.inputPlaceholder')}
          </div>
          <Input
            value={inputPlaceholder}
            maxLength={AGENT_INPUT_PLACEHOLDER_MAX_LENGTH}
            showCharacterCount
            placeholder={defaultInputPlaceholder}
            disabled={readOnly}
            onChange={event =>
              onChangeInputPlaceholder(
                Array.from(event.target.value).slice(0, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH).join('')
              )
            }
          />
        </div>
      </div>

      <div className="space-y-3 pt-2">
        <div className="space-y-2">
          <div className="space-y-1">
            <div className="text-xs font-semibold text-muted-foreground">
              {t('experience.questionsGroup')}
            </div>
            <div className="text-xs leading-5 text-muted-foreground">{t('suggestions.help')}</div>
          </div>
          <AgentSuggestedQuestionsEditor
            questions={normalizedSuggestedQuestions}
            isGeneratingSuggestions={isGeneratingSuggestions}
            readOnly={readOnly}
            onGenerateSuggestedQuestions={onGenerateSuggestedQuestions}
            onChangeSuggestedQuestions={onChangeSuggestedQuestions}
          />
        </div>
      </div>
    </RuntimeSection>
  );
}
