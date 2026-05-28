'use client';

import { Loader2, Plus, Sparkles, Trash2 } from 'lucide-react';
import type { ModelSelectorParameterValue } from '@/components/common/model-selector';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useT } from '@/i18n';
import { AGENT_HOME_TITLE_MAX_LENGTH, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH } from '../constants';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeExperienceSectionProps {
  open: boolean;
  homeTitle: string;
  inputPlaceholder: string;
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  systemPrompt: string;
  modelValue: ModelSelectorParameterValue;
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
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
  systemPrompt,
  modelValue,
  defaultHomeTitle,
  defaultInputPlaceholder,
  onToggleSection,
  onChangeHomeTitle,
  onChangeInputPlaceholder,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
}: AgentRuntimeExperienceSectionProps) {
  const t = useT('agents.agentRuntime');
  const canGenerateSuggestions =
    !isGeneratingSuggestions && Boolean(systemPrompt.trim()) && Boolean(modelValue.model);

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
            onChange={event =>
              onChangeInputPlaceholder(
                Array.from(event.target.value)
                  .slice(0, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH)
                  .join('')
              )
            }
          />
        </div>
      </div>

      <div className="space-y-3 pt-2">
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-xs font-semibold text-muted-foreground">
              {t('experience.questionsGroup')}
            </div>
            <div className="mt-0.5 text-xs text-muted-foreground">{t('suggestions.help')}</div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1.5 px-2 text-xs"
              onClick={onGenerateSuggestedQuestions}
              disabled={!canGenerateSuggestions}
            >
              {isGeneratingSuggestions ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <Sparkles className="size-3.5" />
              )}
              {t('suggestions.generate')}
            </Button>
            <Button
              isIcon
              variant="outline"
              className="size-8"
              onClick={() =>
                onChangeSuggestedQuestions(
                  suggestedQuestions.length >= 6 ? suggestedQuestions : [...suggestedQuestions, '']
                )
              }
              disabled={suggestedQuestions.length >= 6}
              aria-label={t('suggestions.add')}
              title={t('suggestions.add')}
            >
              <Plus className="size-4" />
            </Button>
          </div>
        </div>
        {suggestedQuestions.length === 0 ? (
          <div className="space-y-3 rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            <div>{t('suggestions.empty')}</div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 px-2 text-xs"
                onClick={onGenerateSuggestedQuestions}
                disabled={!canGenerateSuggestions}
              >
                {isGeneratingSuggestions ? (
                  <Loader2 className="size-3.5 animate-spin" />
                ) : (
                  <Sparkles className="size-3.5" />
                )}
                {t('suggestions.generate')}
              </Button>
              <Button
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 px-2 text-xs"
                onClick={() => onChangeSuggestedQuestions([''])}
              >
                <Plus className="size-3.5" />
                {t('suggestions.manualAdd')}
              </Button>
            </div>
          </div>
        ) : (
          suggestedQuestions.map((question, index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                value={question}
                maxLength={200}
                placeholder={t('suggestions.placeholder')}
                onChange={event =>
                  onChangeSuggestedQuestions(
                    suggestedQuestions.map((item, itemIndex) =>
                      itemIndex === index ? event.target.value : item
                    )
                  )
                }
              />
              <Button
                isIcon
                variant="ghost"
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() =>
                  onChangeSuggestedQuestions(
                    suggestedQuestions.filter((_, itemIndex) => itemIndex !== index)
                  )
                }
                aria-label={t('suggestions.delete')}
                title={t('suggestions.delete')}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))
        )}
      </div>
    </RuntimeSection>
  );
}
