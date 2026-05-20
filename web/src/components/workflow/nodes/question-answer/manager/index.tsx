'use client';

import React from 'react';
import { Plus, Trash2 } from 'lucide-react';

import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import NodeValueSelector from '@/components/workflow/common/node-value-selector';
import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '@/components/workflow/common/workflow-value-editor';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type { VariableInsertValue } from '@/components/workflow/common/workflow-value-inserter/variable-item';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import {
  createQuestionAnswerExtractionField,
  createQuestionAnswerChoice,
  normalizeQuestionAnswerNodeData,
  type QuestionAnswerChoice,
  type QuestionAnswerChoiceMode,
  type QuestionAnswerExtractionField,
  type QuestionAnswerNodeData,
  type QuestionAnswerType,
} from '../config';

interface QuestionAnswerManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

function Section({
  title,
  children,
  className,
}: {
  title: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('space-y-3', className)}>
      <h3 className="text-sm font-semibold text-foreground">{title}</h3>
      {children}
    </section>
  );
}

export default function QuestionAnswerManager({
  id,
  className,
  readOnly = false,
}: QuestionAnswerManagerProps) {
  const t = useT('nodes');
  const questionEditorRef = React.useRef<WorkflowValueEditorHandle | null>(null);
  const rawData = useNodeData<QuestionAnswerNodeData>(id);
  const updateData = useNodeDataUpdate<QuestionAnswerNodeData>(id);
  const data = React.useMemo(() => normalizeQuestionAnswerNodeData(rawData), [rawData]);
  const outputVariables = useNodeOutputVariables(id);

  const patchData = React.useCallback(
    (patch: Partial<QuestionAnswerNodeData>) => {
      if (readOnly) return;
      updateData(patch);
    },
    [readOnly, updateData]
  );

  const insertQuestionVariable = React.useCallback(
    (value: VariableInsertValue) => {
      if (readOnly) return;
      questionEditorRef.current?.insertToken(value.sourceId, value.key);
    },
    [readOnly]
  );

  const updateChoice = React.useCallback(
    (index: number, patch: Partial<QuestionAnswerChoice>) => {
      if (readOnly) return;
      const choices = data.choices.map((choice, choiceIndex) =>
        choiceIndex === index ? { ...choice, ...patch } : choice
      );
      updateData({ choices });
    },
    [data.choices, readOnly, updateData]
  );

  const removeChoice = React.useCallback(
    (index: number) => {
      if (readOnly) return;
      updateData({ choices: data.choices.filter((_, choiceIndex) => choiceIndex !== index) });
    },
    [data.choices, readOnly, updateData]
  );

  const addChoice = React.useCallback(() => {
    if (readOnly) return;
    updateData({ choices: [...data.choices, createQuestionAnswerChoice(data.choices)] });
  }, [data.choices, readOnly, updateData]);

  const updateExtractionField = React.useCallback(
    (index: number, patch: Partial<QuestionAnswerExtractionField>) => {
      if (readOnly) return;
      const extractionFields = data.extraction_fields.map((field, fieldIndex) =>
        fieldIndex === index ? { ...field, ...patch } : field
      );
      updateData({ extraction_fields: extractionFields });
    },
    [data.extraction_fields, readOnly, updateData]
  );

  const removeExtractionField = React.useCallback(
    (index: number) => {
      if (readOnly) return;
      updateData({
        extraction_fields: data.extraction_fields.filter((_, fieldIndex) => fieldIndex !== index),
      });
    },
    [data.extraction_fields, readOnly, updateData]
  );

  const addExtractionField = React.useCallback(() => {
    if (readOnly) return;
    updateData({
      extraction_fields: [
        ...data.extraction_fields,
        createQuestionAnswerExtractionField(data.extraction_fields),
      ],
    });
  }, [data.extraction_fields, readOnly, updateData]);

  const setAnswerType = React.useCallback(
    (answerType: QuestionAnswerType) => {
      patchData({ answer_type: answerType });
    },
    [patchData]
  );

  const setChoiceMode = React.useCallback(
    (choiceMode: QuestionAnswerChoiceMode) => {
      patchData({
        choice_mode: choiceMode,
        ...(choiceMode === 'static'
          ? {
              dynamic_choices: { selector: [] },
            }
          : {}),
      });
    },
    [patchData]
  );

  return (
    <div className={cn('space-y-5', className)}>
      <Section title={t('questionAnswer.manager.question')}>
        <WorkflowValueInserter
          nodeId={id}
          className="w-full"
          onInsert={insertQuestionVariable}
          disabled={readOnly}
        />
        <WorkflowValueEditor
          ref={questionEditorRef}
          value={data.question}
          readOnly={readOnly}
          placeholder={t('questionAnswer.placeholders.question')}
          nodeId={id}
          editorClassName="min-h-24 max-h-[260px] overflow-y-auto scrollbar-thin"
          onChange={question => patchData({ question })}
        />
      </Section>

      <Section title={t('questionAnswer.manager.answerType')}>
        <Select value={data.answer_type} disabled={readOnly} onValueChange={setAnswerType}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="text">{t('questionAnswer.answerTypes.text')}</SelectItem>
            <SelectItem value="choice">{t('questionAnswer.answerTypes.choice')}</SelectItem>
          </SelectContent>
        </Select>
      </Section>

      {data.answer_type === 'text' ? (
        <>
          <Section title={t('questionAnswer.manager.extractFromAnswer')}>
            <div className="flex items-center justify-between gap-3 rounded-md border px-3 py-2">
              <Label className="text-sm font-normal text-foreground">
                {t('questionAnswer.manager.extractFromAnswerDescription')}
              </Label>
              <Switch
                checked={data.extract_from_answer}
                disabled={readOnly}
                onCheckedChange={checked => patchData({ extract_from_answer: Boolean(checked) })}
              />
            </div>
          </Section>

          {data.extract_from_answer ? (
            <>
              <Section title={t('questionAnswer.manager.model')}>
                <ModelSelectorParameter
                  modelType="text-chat"
                  disabled={readOnly}
                  value={{
                    provider: data.model.provider,
                    model: data.model.name || data.model.model || '',
                    params: data.model.completion_params || {},
                  }}
                  onChange={value => {
                    patchData({
                      model: {
                        provider: value.provider,
                        name: value.model,
                        completion_params: value.params,
                      },
                      model_config: {
                        provider: value.provider,
                        name: value.model,
                        completion_params: value.params,
                      },
                    });
                  }}
                />
              </Section>

              <Section title={t('questionAnswer.manager.extractionInstruction')}>
                <Textarea
                  value={data.extraction_instruction}
                  disabled={readOnly}
                  placeholder={t('questionAnswer.placeholders.extractionInstruction')}
                  className="min-h-24 resize-none"
                  onChange={event =>
                    patchData({
                      extraction_instruction: event.target.value,
                      completion_instruction: event.target.value,
                    })
                  }
                />
              </Section>

              <Section title={t('questionAnswer.manager.extractionFields')}>
                <div className="space-y-2">
                  {data.extraction_fields.map((field, index) => (
                    <div key={`${field.name}-${index}`} className="space-y-2 rounded-md border p-2">
                      <div className="grid grid-cols-[minmax(0,1fr)_116px_32px] gap-2">
                        <Input
                          value={field.name}
                          disabled={readOnly}
                          placeholder={t('questionAnswer.placeholders.extractionFieldName')}
                          onChange={event => updateExtractionField(index, { name: event.target.value })}
                        />
                        <Select
                          value={field.type}
                          disabled={readOnly}
                          onValueChange={value =>
                            updateExtractionField(index, {
                              type: value as QuestionAnswerExtractionField['type'],
                            })
                          }
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="string">String</SelectItem>
                            <SelectItem value="number">Number</SelectItem>
                            <SelectItem value="boolean">Boolean</SelectItem>
                          </SelectContent>
                        </Select>
                        <Button
                          type="button"
                          variant="ghost"
                          isIcon
                          disabled={readOnly}
                          onClick={() => removeExtractionField(index)}
                          aria-label={t('common.remove')}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      </div>
                      <Input
                        value={field.description}
                        disabled={readOnly}
                        placeholder={t('questionAnswer.placeholders.extractionFieldDescription')}
                        onChange={event =>
                          updateExtractionField(index, { description: event.target.value })
                        }
                      />
                      <label className="flex items-center gap-2 text-xs text-muted-foreground">
                        <Checkbox
                          checked={field.required}
                          disabled={readOnly}
                          onCheckedChange={checked =>
                            updateExtractionField(index, { required: Boolean(checked) })
                          }
                        />
                        {t('questionAnswer.manager.requiredField')}
                      </label>
                    </div>
                  ))}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={readOnly}
                    onClick={addExtractionField}
                  >
                    <Plus className="size-4" />
                    {t('questionAnswer.manager.addExtractionField')}
                  </Button>
                </div>
              </Section>

              <Section title={t('questionAnswer.manager.maxAnswerCount')}>
                <Input
                  type="number"
                  min={1}
                  max={10}
                  value={data.max_answer_count}
                  disabled={readOnly}
                  onChange={event => {
                    const next = Number(event.target.value);
                    patchData({ max_answer_count: Number.isFinite(next) ? next : 3 });
                  }}
                />
              </Section>
            </>
          ) : null}
        </>
      ) : (
        <Section title={t('questionAnswer.manager.choices')}>
          <Select value={data.choice_mode} disabled={readOnly} onValueChange={setChoiceMode}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="static">{t('questionAnswer.choiceModes.static')}</SelectItem>
              <SelectItem value="dynamic">{t('questionAnswer.choiceModes.dynamic')}</SelectItem>
            </SelectContent>
          </Select>

          {data.choice_mode === 'dynamic' ? (
            <NodeValueSelector
              nodeId={id}
              value={data.dynamic_choices.selector}
              disabled={readOnly}
              label={t('questionAnswer.manager.dynamicSelector')}
              placeholder={t('questionAnswer.placeholders.dynamicSelector')}
              typeFilter={type => type === 'array' || type === 'array[string]' || type === 'array[object]'}
              onChange={value => {
                patchData({ dynamic_choices: { selector: value.valuePath } });
              }}
            />
          ) : (
            <div className="space-y-2">
              {data.choices.map((choice, index) => (
                <div key={`${choice.id}-${index}`} className="grid grid-cols-[72px_1fr_32px] gap-2">
                  <Input
                    value={choice.id}
                    disabled={readOnly}
                    placeholder={t('questionAnswer.placeholders.choiceId')}
                    onChange={event => {
                      const idValue = event.target.value;
                      updateChoice(index, {
                        id: idValue,
                        value: choice.value === choice.id ? idValue : choice.value,
                      });
                    }}
                  />
                  <Input
                    value={choice.label}
                    disabled={readOnly}
                    placeholder={t('questionAnswer.placeholders.choiceLabel')}
                    onChange={event => updateChoice(index, { label: event.target.value })}
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    isIcon
                    disabled={readOnly || data.choices.length <= 1}
                    onClick={() => removeChoice(index)}
                    aria-label={t('common.remove')}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ))}
              <Button type="button" variant="outline" size="sm" disabled={readOnly} onClick={addChoice}>
                <Plus className="size-4" />
                {t('questionAnswer.manager.addChoice')}
              </Button>
            </div>
          )}
        </Section>
      )}

      <OutputVariablesView variables={outputVariables} />
    </div>
  );
}
