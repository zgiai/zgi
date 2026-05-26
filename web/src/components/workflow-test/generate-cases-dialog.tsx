'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useCreateWorkflowTestGenerationTask } from '@/hooks/workflow-test/use-workflow-test';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { useT } from '@/i18n';

interface GenerateCasesDialogProps {
  agentId: string;
  scenarios: Array<{ id: string; name: string }>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onGenerationStart?: (count: number) => void;
  onGenerationCreateFailed?: () => void;
}

const MIN_GENERATED_CASE_COUNT = 1;
const MAX_GENERATED_CASE_COUNT = 50;
const COUNT_PRESETS = [10, 20, MAX_GENERATED_CASE_COUNT];
const QUESTION_TYPES = [
  { value: 'core', labelKey: 'core' as const },
  { value: 'extension', labelKey: 'extension' as const },
  { value: 'fuzzy', labelKey: 'fuzzy' as const },
];
const TURN_STRATEGIES = [
  { value: 'mixed' as const, labelKey: 'turnStrategyMixed' as const },
  { value: 'single' as const, labelKey: 'turnStrategySingle' as const },
  { value: 'multi' as const, labelKey: 'turnStrategyMulti' as const },
];

export function GenerateCasesDialog({
  agentId,
  scenarios,
  open,
  onOpenChange,
  onGenerationStart,
  onGenerationCreateFailed,
}: GenerateCasesDialogProps) {
  const t = useT('agents.workflowTest.dialogs.generateCases');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const createGenerationTask = useCreateWorkflowTestGenerationTask(agentId);
  const user = useCurrentUser();
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const [count, setCount] = React.useState(20);
  const [scenarioIds, setScenarioIds] = React.useState<string[]>([]);
  const [questionTypes, setQuestionTypes] = React.useState<string[]>(['core', 'extension', 'fuzzy']);
  const [turnStrategy, setTurnStrategy] = React.useState<'mixed' | 'single' | 'multi'>('mixed');
  const [model, setModel] = React.useState<ModelSelectorValue | null>(null);
  const [prompt, setPrompt] = React.useState('');
  const [context, setContext] = React.useState('');

  React.useEffect(() => {
    if (!user?.id) return;
    const saved = getLastSelectedAiModel(user.id, 'workflowTestScenario');
    if (saved) {
      setModel({ provider: saved.provider, model: saved.model });
      return;
    }
    if (defaultModel) {
      setModel({ provider: defaultModel.provider, model: defaultModel.model });
    }
  }, [defaultModel, user?.id]);

  React.useEffect(() => {
    if (open) return;
    setCount(20);
    setScenarioIds([]);
    setQuestionTypes(['core', 'extension', 'fuzzy']);
    setTurnStrategy('mixed');
    setPrompt('');
    setContext('');
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    setScenarioIds(prev => {
      if (prev.length > 0 || scenarios.length === 0) return prev;
      return scenarios.map(scene => scene.id);
    });
    setPrompt(prev => prev || t('promptDefault'));
  }, [open, scenarios, t]);

  const safeCount = Number.isFinite(count) ? count : 0;
  const canSubmit =
    safeCount >= MIN_GENERATED_CASE_COUNT &&
    safeCount <= MAX_GENERATED_CASE_COUNT &&
    scenarioIds.length > 0 &&
    questionTypes.length > 0 &&
    Boolean(model?.provider && model?.model) &&
    !createGenerationTask.isPending;

  const toggleScenario = (id: string, checked: boolean) => {
    setScenarioIds(prev =>
      checked ? Array.from(new Set([...prev, id])) : prev.filter(sceneId => sceneId !== id)
    );
  };

  const toggleQuestionType = (value: string, checked: boolean) => {
    setQuestionTypes(prev =>
      checked ? Array.from(new Set([...prev, value])) : prev.filter(item => item !== value)
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="w-[calc(100vw-32px)] max-w-[800px] rounded-2xl">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[calc(100vh-220px)] space-y-6 overflow-y-auto pr-1">
          <section className="space-y-3">
            <Label>{t('countLabel')}</Label>
            <div className="flex flex-wrap gap-2">
              {COUNT_PRESETS.map(preset => (
                <Button
                  key={preset}
                  type="button"
                  variant={count === preset ? 'default' : 'outline'}
                  size="sm"
                  onClick={() => setCount(preset)}
                >
                  {preset}
                </Button>
              ))}
              <div className="w-24">
                <Input
                  type="number"
                  min={MIN_GENERATED_CASE_COUNT}
                  max={MAX_GENERATED_CASE_COUNT}
                  value={count}
                  onChange={event => {
                    const next = Number(event.target.value);
                    if (!Number.isFinite(next)) {
                      setCount(MIN_GENERATED_CASE_COUNT);
                      return;
                    }
                    setCount(Math.min(MAX_GENERATED_CASE_COUNT, Math.max(MIN_GENERATED_CASE_COUNT, next)));
                  }}
                />
              </div>
            </div>
          </section>

          <section className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>{t('scenarioLabel')}</Label>
              <div className="flex items-center gap-3 text-sm text-slate-500">
                <button
                  type="button"
                  className="font-medium"
                  onClick={() => setScenarioIds(scenarios.map(scene => scene.id))}
                >
                  {commonT('selectAll')}
                </button>
                <button type="button" className="font-medium" onClick={() => setScenarioIds([])}>
                  {commonT('clearSelection')}
                </button>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {scenarios.map(scene => {
                const checked = scenarioIds.includes(scene.id);
                return (
                  <label
                    key={scene.id}
                    className="flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors"
                  >
                    <Checkbox checked={checked} onCheckedChange={next => toggleScenario(scene.id, Boolean(next))} />
                    <span>{scene.name}</span>
                  </label>
                );
              })}
            </div>
          </section>

          <section className="space-y-3">
            <Label>{t('questionTypeLabel')}</Label>
            <p className="text-sm text-slate-500">{t('questionTypeDescription')}</p>
            <div className="flex flex-wrap gap-2">
              {QUESTION_TYPES.map(item => {
                const checked = questionTypes.includes(item.value);
                return (
                  <label
                    key={item.value}
                    className="flex cursor-pointer items-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors"
                  >
                    <Checkbox
                      checked={checked}
                      onCheckedChange={next => toggleQuestionType(item.value, Boolean(next))}
                    />
                    <span>{typeT(item.labelKey)}</span>
                  </label>
                );
              })}
            </div>
          </section>

          <section className="space-y-3">
            <Label>{t('turnStrategyLabel')}</Label>
            <div className="grid grid-cols-3 gap-3">
              {TURN_STRATEGIES.map(item => (
                <Button
                  key={item.value}
                  type="button"
                  variant={turnStrategy === item.value ? 'default' : 'outline'}
                  className="h-12"
                  onClick={() => setTurnStrategy(item.value)}
                >
                  {t(item.labelKey)}
                </Button>
              ))}
            </div>
          </section>

          <section className="space-y-3">
            <Label>{t('modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={model ?? undefined}
              onChange={value => {
                setModel(value);
                if (user?.id) {
                  saveLastSelectedAiModel(user.id, 'workflowTestScenario', {
                    provider: value.provider,
                    model: value.model,
                  });
                }
              }}
              placeholder={t('modelPlaceholder')}
            />
          </section>

          <section className="space-y-3">
            <Label htmlFor="workflow-test-generate-prompt">{t('promptLabel')}</Label>
            <Textarea
              id="workflow-test-generate-prompt"
              value={prompt}
              onChange={event => setPrompt(event.target.value)}
              placeholder={t('promptPlaceholder')}
              className="min-h-48 resize-none leading-7"
            />
          </section>

          <section className="space-y-3">
            <Label htmlFor="workflow-test-generate-context">{t('contextLabel')}</Label>
            <Textarea
              id="workflow-test-generate-context"
              value={context}
              onChange={event => setContext(event.target.value)}
              placeholder={t('contextPlaceholder')}
              className="min-h-24 resize-none"
            />
          </section>

          <div className="flex flex-wrap gap-2">
            {scenarioIds.length > 0 ? (
              <Badge variant="outline">{t('selectedScenarioCount', { count: scenarioIds.length })}</Badge>
            ) : null}
            {questionTypes.length > 0 ? (
              <Badge variant="outline">
                {t('selectedQuestionTypeCount', { count: questionTypes.length })}
              </Badge>
            ) : null}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            disabled={!canSubmit}
            onClick={() => {
              if (!model) return;
              const payload = {
                count: safeCount,
                scenario_ids: scenarioIds,
                question_types: questionTypes,
                turn_strategy: turnStrategy,
                prompt,
                context,
                model: {
                  provider: model.provider,
                  name: model.model,
                },
              };
              onGenerationStart?.(safeCount);
              onOpenChange(false);
              createGenerationTask.mutate(
                payload,
                { onError: () => onGenerationCreateFailed?.() }
              );
            }}
          >
            {createGenerationTask.isPending ? t('submitting') : t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
