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
import { Textarea } from '@/components/ui/textarea';
import { ModelSelector } from '@/components/common/model-selector';
import type { ModelSelectorValue } from '@/components/common/model-selector';
import { useDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useT } from '@/i18n';
import {
  useResetWorkflowTestJudgePrompt,
  useUpdateWorkflowTestSettings,
  useWorkflowTestSettings,
} from '@/hooks/workflow-test/use-workflow-test';

interface JudgePromptSettingsDialogProps {
  agentId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function JudgePromptSettingsDialog({
  agentId,
  open,
  onOpenChange,
}: JudgePromptSettingsDialogProps) {
  const t = useT('agents.workflowTest.dialogs.judgeSettings');
  const commonT = useT('agents.workflowTest.common');
  const { data, isLoading } = useWorkflowTestSettings(agentId);
  const { value: defaultModel } = useDefaultModelByUseCase('text-chat');
  const updateSettings = useUpdateWorkflowTestSettings(agentId);
  const resetPrompt = useResetWorkflowTestJudgePrompt(agentId);
  const [prompt, setPrompt] = React.useState('');
  const [selectedModel, setSelectedModel] = React.useState<ModelSelectorValue | null>(null);

  React.useEffect(() => {
    if (!open || !data?.data) return;
    setPrompt(data.data.judge_prompt_template);
    if (data.data.judge_model_provider && data.data.judge_model_name) {
      setSelectedModel({
        provider: data.data.judge_model_provider,
        model: data.data.judge_model_name,
      });
    } else if (defaultModel) {
      setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
    }
  }, [data?.data, defaultModel, open]);

  React.useEffect(() => {
    if (open && !selectedModel && defaultModel) {
      setSelectedModel({ provider: defaultModel.provider, model: defaultModel.model });
    }
  }, [defaultModel, open, selectedModel]);

  const isSaving = updateSettings.isPending || resetPrompt.isPending;
  const canSave = Boolean(prompt.trim() && selectedModel?.provider && selectedModel?.model);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl">
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="space-y-2">
            <Label>{t('modelLabel')}</Label>
            <ModelSelector
              modelType="text-chat"
              value={selectedModel ?? undefined}
              disabled={isLoading || isSaving}
              onChange={setSelectedModel}
              placeholder={t('modelPlaceholder')}
            />
          </div>
          <div className="rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
            {t('warning')}
          </div>
          <div className="space-y-2">
            <Label htmlFor="workflow-test-judge-prompt">{t('label')}</Label>
            <Textarea
              id="workflow-test-judge-prompt"
              value={prompt}
              disabled={isLoading || isSaving}
              onChange={event => setPrompt(event.target.value)}
              className="min-h-[320px] resize-none font-mono text-sm leading-6"
              placeholder={t('placeholder')}
            />
          </div>
        </DialogBody>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={isSaving}
            onClick={() => {
              resetPrompt.mutate(undefined, {
                onSuccess: response => {
                  setPrompt(response.data.judge_prompt_template);
                  if (defaultModel) {
                    setSelectedModel({
                      provider: defaultModel.provider,
                      model: defaultModel.model,
                    });
                  }
                },
              });
            }}
          >
            {t('reset')}
          </Button>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {commonT('cancel')}
          </Button>
          <Button
            type="button"
            disabled={isSaving || !canSave}
            onClick={() => {
              if (!selectedModel) return;
              updateSettings.mutate(
                {
                  judge_prompt_template: prompt,
                  judge_model_provider: selectedModel.provider,
                  judge_model_name: selectedModel.model,
                },
                { onSuccess: () => onOpenChange(false) }
              );
            }}
          >
            {t('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
