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
import { useGenerateWorkflowTestCases } from '@/hooks/workflow-test/use-workflow-test';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { useT } from '@/i18n';

interface GenerateCasesDialogProps {
  agentId: string;
  scenarios: Array<{ id: string; name: string }>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const COUNT_PRESETS = [10, 20, 50];
const QUESTION_TYPES = [
  { value: 'core', labelKey: 'core' as const },
  { value: 'extension', labelKey: 'extension' as const },
  { value: 'fuzzy', labelKey: 'fuzzy' as const },
];
const TURN_STRATEGIES = [
  { value: 'mixed' as const, label: '单轮 + 多轮' },
  { value: 'single' as const, label: '单轮对话' },
  { value: 'multi' as const, label: '多轮对话' },
];

function defaultPrompt(): string {
  return `请基于当前智能体工作流、已识别业务场景和已有测试问题，生成一批可进入问题库的候选测试问题。

生成要求：
1. 每个问题都要模拟真实用户表达，避免像测试脚本。
2. 结合所选业务场景生成问题，优先覆盖高频、关键、异常和兜底场景。
3. 结合所选问题类型生成不同复杂度的问题。
4. 如果选择了多轮对话，请生成有上下文衔接的对话输入。
5. 为每个问题补充预期结果，描述智能体应如何正确回答。
6. 输出 JSON 对象，格式为：{"cases":[{"content":"问题内容","expected_result":"预期结果","question_type":"core"}]}`;
}

export function GenerateCasesDialog({
  agentId,
  scenarios,
  open,
  onOpenChange,
}: GenerateCasesDialogProps) {
  const t = useT('agents.workflowTest.dialogs.generateCases');
  const commonT = useT('agents.workflowTest.common');
  const typeT = useT('agents.workflowTest.questionTypes');
  const generateCases = useGenerateWorkflowTestCases(agentId);
  const user = useCurrentUser();
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const [count, setCount] = React.useState(20);
  const [scenarioIds, setScenarioIds] = React.useState<string[]>([]);
  const [questionTypes, setQuestionTypes] = React.useState<string[]>(['core', 'extension', 'fuzzy']);
  const [turnStrategy, setTurnStrategy] = React.useState<'mixed' | 'single' | 'multi'>('mixed');
  const [model, setModel] = React.useState<ModelSelectorValue | null>(null);
  const [prompt, setPrompt] = React.useState(defaultPrompt());
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
    if (!open) {
      setCount(20);
      setScenarioIds([]);
      setQuestionTypes(['core', 'extension', 'fuzzy']);
      setTurnStrategy('mixed');
      setPrompt(defaultPrompt());
      setContext('');
    } else if (scenarios.length > 0 && scenarioIds.length === 0) {
      setScenarioIds(scenarios.map(scene => scene.id));
    }
  }, [open, scenarios, scenarioIds.length]);

  const safeCount = Number.isFinite(count) ? count : 0;
  const canSubmit =
    safeCount >= 1 &&
    safeCount <= 50 &&
    scenarioIds.length > 0 &&
    questionTypes.length > 0 &&
    Boolean(model?.provider && model?.model) &&
    !generateCases.isPending;

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
      <DialogContent size="lg" className="max-w-[1180px] rounded-2xl">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-6">
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
                  min={1}
                  max={50}
                  value={count}
                  onChange={event => setCount(Number(event.target.value))}
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
                  {item.label}
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
              generateCases.mutate(
                {
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
                },
                { onSuccess: () => onOpenChange(false) }
              );
            }}
          >
            {generateCases.isPending ? t('submitting') : t('submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
