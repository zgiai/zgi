'use client';

import { useState } from 'react';
import { Settings2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ChatOpeningGuideView } from '@/components/chat/ui/chat-opening-guide-view';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import OpeningStatementDialog, {
  type OpeningStatementDialogValue,
} from '@/components/workflow/ui/features-panel/opening-statement-dialog';
import { SUGGESTED_QUESTIONS_LIMIT } from '@/constants/suggested-questions';
import { useT } from '@/i18n';
import { AGENT_INPUT_PLACEHOLDER_MAX_LENGTH } from '../constants';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface GenerateSuggestedQuestionsResult {
  questions: string[];
  warnings?: string[];
}

interface AgentRuntimeExperienceSectionProps {
  open: boolean;
  homeTitle: string;
  openingStatement: string;
  inputPlaceholder: string;
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  openingGuideBrand?: OpeningGuideBrand;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeOpeningStatement: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onGenerateSuggestedQuestions: (
    value: OpeningStatementDialogValue
  ) => Promise<GenerateSuggestedQuestionsResult | undefined>;
  onChangeSuggestedQuestions: (value: string[]) => void;
}

export function AgentRuntimeExperienceSection({
  open,
  homeTitle,
  openingStatement,
  inputPlaceholder,
  suggestedQuestions,
  isGeneratingSuggestions,
  defaultHomeTitle,
  defaultInputPlaceholder,
  openingGuideBrand,
  readOnly = false,
  onToggleSection,
  onChangeHomeTitle,
  onChangeOpeningStatement,
  onChangeInputPlaceholder,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
}: AgentRuntimeExperienceSectionProps) {
  const t = useT('agents.agentRuntime');
  const tAgents = useT('agents');
  const [openingDialogOpen, setOpeningDialogOpen] = useState(false);
  const normalizedSuggestedQuestions = suggestedQuestions
    .map(question => question.trim())
    .filter(Boolean)
    .slice(0, SUGGESTED_QUESTIONS_LIMIT);
  const resolvedHomeTitle = homeTitle.trim() || defaultHomeTitle;

  return (
    <RuntimeSection
      title={t('sections.experience')}
      section="experience"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="space-y-1">
            <div className="text-xs font-semibold text-muted-foreground">
              {tAgents('workflow.features.openingStatement.label')}
            </div>
            <p className="text-xs leading-5 text-muted-foreground">
              {tAgents('workflow.features.openingStatement.desc')}
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="xs"
            className="shrink-0 gap-1.5 bg-background"
            disabled={readOnly}
            onClick={() => setOpeningDialogOpen(true)}
          >
            <Settings2 className="size-3.5" />
            {tAgents('workflow.features.openingStatement.dialogTitle')}
          </Button>
        </div>

        <div className="max-h-80 overflow-y-auto rounded-lg border border-border/70 bg-muted/20 px-3 py-4">
          <ChatOpeningGuideView
            title={resolvedHomeTitle}
            message={openingStatement}
            iconType={openingGuideBrand?.iconType}
            icon={openingGuideBrand?.icon}
            iconBackground={openingGuideBrand?.iconBackground}
            iconSrc={openingGuideBrand?.iconSrc}
            suggestions={normalizedSuggestedQuestions}
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

      <OpeningStatementDialog
        open={openingDialogOpen}
        onOpenChange={setOpeningDialogOpen}
        value={{
          title: homeTitle,
          message: openingStatement,
          suggestedQuestions: normalizedSuggestedQuestions,
        }}
        onSave={value => {
          onChangeHomeTitle(value.title);
          onChangeOpeningStatement(value.message);
          onChangeSuggestedQuestions(value.suggestedQuestions);
        }}
        onGenerateSuggestedQuestions={onGenerateSuggestedQuestions}
        generatingSuggestedQuestions={isGeneratingSuggestions}
        previewBrand={openingGuideBrand}
      />
    </RuntimeSection>
  );
}
