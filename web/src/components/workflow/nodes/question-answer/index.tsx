'use client';

import React from 'react';
import { Position } from '@xyflow/react';

import CustomHandle from '../../ui/custom-handle';
import OutputVariablesView from '../../common/output-variables-view';
import ValueBadge from '../../ui/value-badge';
import { useNodeOutputVariables } from '../../hooks';
import { useT } from '@/i18n';

import {
  normalizeQuestionAnswerNodeData,
  QUESTION_ANSWER_DYNAMIC_HANDLE,
  getQuestionAnswerOutputVariables,
  type QuestionAnswerChoice,
  type QuestionAnswerNodeData,
} from './config';

interface QuestionAnswerContentProps {
  nodeId: string;
  data: QuestionAnswerNodeData;
}

function SummaryRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[64px_minmax(0,1fr)] items-start gap-2 text-xs">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <div className="min-w-0 text-secondary-foreground">{children}</div>
    </div>
  );
}

function ChoiceRow({ choice, index }: { choice: QuestionAnswerChoice; index: number }) {
  return (
    <div className="relative flex items-center gap-2 py-1 text-xs">
      <span className="inline-flex h-6 min-w-9 shrink-0 items-center justify-center rounded-md bg-muted px-2 font-mono font-semibold text-foreground">
        {choice.id || index + 1}
      </span>
      <span className="min-w-0 flex-1 truncate text-secondary-foreground">
        {choice.label || choice.value || choice.id}
      </span>
      <CustomHandle
        type="source"
        position={Position.Right}
        id={choice.id}
        style={{ top: '50%', right: -15 }}
      />
    </div>
  );
}

export default function QuestionAnswerContent({ nodeId, data }: QuestionAnswerContentProps) {
  const t = useT('nodes');
  const normalized = normalizeQuestionAnswerNodeData(data);
  const storeOutputVariables = useNodeOutputVariables(nodeId);
  const outputVariables = storeOutputVariables.length
    ? storeOutputVariables
    : getQuestionAnswerOutputVariables(normalized).map(variable => ({
        name: variable.key,
        type: variable.type,
      }));
  const isChoice = normalized.answer_type === 'choice';
  const isDynamic = isChoice && normalized.choice_mode === 'dynamic';

  return (
    <div className="space-y-2">
      <OutputVariablesView
        variant="compact"
        variables={outputVariables}
        maxItems={3}
        showCount={false}
        expandHiddenItems
      />

      <div className="space-y-1.5">
        <SummaryRow label={t('questionAnswer.canvas.question')}>
          {normalized.question.trim() ? (
            <span className="line-clamp-2 break-words">{normalized.question}</span>
          ) : (
            <span className="text-muted-foreground">
              {t('questionAnswer.canvas.emptyQuestion')}
            </span>
          )}
        </SummaryRow>
        <SummaryRow label={t('questionAnswer.canvas.answerType')}>
          {isChoice ? t('questionAnswer.answerTypes.choice') : t('questionAnswer.answerTypes.text')}
        </SummaryRow>
        {!isChoice && normalized.extract_from_answer ? (
          <SummaryRow label={t('questionAnswer.manager.extractFromAnswer')}>
            <span className="text-secondary-foreground">
              {normalized.extraction_fields.length || 0}
            </span>
          </SummaryRow>
        ) : null}
      </div>

      {isChoice ? (
        <div className="space-y-1 pl-[64px]">
          {isDynamic ? (
            <>
              <div className="relative flex items-center gap-2 py-1 text-xs">
                <span className="inline-flex h-6 min-w-12 shrink-0 items-center justify-center rounded-md bg-muted px-2 font-mono font-semibold text-foreground">
                  A-Z
                </span>
                {normalized.dynamic_choices.selector.length >= 2 ? (
                  <ValueBadge
                    selector={normalized.dynamic_choices.selector}
                    currentNodeId={nodeId}
                    className="min-w-0"
                  />
                ) : (
                  <span className="truncate text-muted-foreground">
                    {t('questionAnswer.dynamicOption')}
                  </span>
                )}
                <CustomHandle
                  type="source"
                  position={Position.Right}
                  id={QUESTION_ANSWER_DYNAMIC_HANDLE}
                  style={{ top: '50%', right: -15 }}
                />
              </div>
            </>
          ) : (
            normalized.choices.slice(0, 6).map((choice, index) => (
              <ChoiceRow key={`${choice.id}-${index}`} choice={choice} index={index} />
            ))
          )}
          {!isDynamic && normalized.choices.length > 6 ? (
            <div className="text-[11px] text-muted-foreground">
              +{normalized.choices.length - 6}
            </div>
          ) : null}
        </div>
      ) : null}

      {!isChoice ? (
        <CustomHandle
          type="source"
          position={Position.Right}
          id="source"
          style={{ top: 20, right: -15 }}
        />
      ) : null}
      <CustomHandle type="target" position={Position.Left} id="target" style={{ top: 20, left: -15 }} />
    </div>
  );
}
