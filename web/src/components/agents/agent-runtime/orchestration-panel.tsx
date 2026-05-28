'use client';

import type { ModelSelectorParameterValue } from '@/components/common/model-selector';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { AgentMemorySlotConfig } from '@/services/types/agent';
import type { Dataset } from '@/services/types/dataset';
import { AgentRuntimeExperienceSection } from './sections/experience-section';
import { AgentRuntimeFileSection } from './sections/file-section';
import { AgentRuntimeKnowledgeSection } from './sections/knowledge-section';
import { AgentRuntimeMemorySection } from './sections/memory-section';
import { AgentRuntimeModelSection } from './sections/model-section';
import { AgentRuntimeSkillSection } from './sections/skill-section';
import type { AgentConfigSection } from './types';
import type { AgentMemorySlotValidationError } from './utils';

interface AgentRuntimeOrchestrationPanelProps {
  locale: string;
  openSections: Record<AgentConfigSection, boolean>;
  modelValue: ModelSelectorParameterValue;
  homeTitle: string;
  inputPlaceholder: string;
  selectedSkills: AIChatSkillMetadata[];
  normalizedSelectedSkillIds: string[];
  selectableSkillsCount: number;
  isSkillsLoading: boolean;
  isSkillConfigLoading: boolean;
  isDatasetsLoading: boolean;
  availableDatasets: Dataset[];
  selectedKnowledgeDatasetIds: string[];
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  systemPrompt: string;
  fileUploadEnabled: boolean;
  agentMemoryEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  className?: string;
  scrollAreaClassName?: string;
  scrollViewportClassName?: string;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onOpenSkillDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
  onGenerateSuggestedQuestions: () => void;
  onChangeSuggestedQuestions: (value: string[]) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
}

export function AgentRuntimeOrchestrationPanel({
  locale,
  openSections,
  modelValue,
  homeTitle,
  inputPlaceholder,
  selectedSkills,
  normalizedSelectedSkillIds,
  selectableSkillsCount,
  isSkillsLoading,
  isSkillConfigLoading,
  isDatasetsLoading,
  availableDatasets,
  selectedKnowledgeDatasetIds,
  suggestedQuestions,
  isGeneratingSuggestions,
  systemPrompt,
  fileUploadEnabled,
  agentMemoryEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  defaultHomeTitle,
  defaultInputPlaceholder,
  className,
  scrollAreaClassName,
  scrollViewportClassName,
  onToggleSection,
  onChangeModelValue,
  onChangeHomeTitle,
  onChangeInputPlaceholder,
  onOpenSkillDialog,
  onToggleSkill,
  onToggleKnowledgeDataset,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
  onChangeFileUploadEnabled,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemorySlots,
}: AgentRuntimeOrchestrationPanelProps) {
  const t = useT('agents.agentRuntime');

  return (
    <section className={cn('flex min-w-0 flex-col overflow-hidden', className)}>
      <div className="flex h-12 shrink-0 items-center justify-between px-5">
        <div>
          <h2 className="text-sm font-semibold">{t('orchestration.title')}</h2>
          {t('orchestration.description') ? (
            <p className="text-xs text-muted-foreground">{t('orchestration.description')}</p>
          ) : null}
        </div>
      </div>
      <ScrollArea
        className={cn('min-h-0 flex-1', scrollAreaClassName)}
        viewportProps={scrollViewportClassName ? { className: scrollViewportClassName } : undefined}
      >
        <div className="space-y-5 px-5 pb-6">
          <AgentRuntimeModelSection
            open={openSections.model}
            modelValue={modelValue}
            onToggleSection={onToggleSection}
            onChangeModelValue={onChangeModelValue}
          />

          <Separator className="h-px" />

          <AgentRuntimeSkillSection
            locale={locale}
            open={openSections.skills}
            selectedSkills={selectedSkills}
            normalizedSelectedSkillIds={normalizedSelectedSkillIds}
            selectableSkillsCount={selectableSkillsCount}
            isSkillsLoading={isSkillsLoading}
            isSkillConfigLoading={isSkillConfigLoading}
            onToggleSection={onToggleSection}
            onOpenSkillDialog={onOpenSkillDialog}
            onToggleSkill={onToggleSkill}
          />

          <Separator className="h-px" />

          <AgentRuntimeKnowledgeSection
            open={openSections.knowledge}
            isDatasetsLoading={isDatasetsLoading}
            availableDatasets={availableDatasets}
            selectedKnowledgeDatasetIds={selectedKnowledgeDatasetIds}
            onToggleSection={onToggleSection}
            onToggleKnowledgeDataset={onToggleKnowledgeDataset}
          />

          <Separator className="h-px" />

          <AgentRuntimeFileSection
            open={openSections.files}
            fileUploadEnabled={fileUploadEnabled}
            onToggleSection={onToggleSection}
            onChangeFileUploadEnabled={onChangeFileUploadEnabled}
          />

          <Separator className="h-px" />

          <AgentRuntimeMemorySection
            open={openSections.memory}
            agentMemoryEnabled={agentMemoryEnabled}
            agentMemorySlots={agentMemorySlots}
            agentMemorySlotValidationErrors={agentMemorySlotValidationErrors}
            onToggleSection={onToggleSection}
            onChangeAgentMemoryEnabled={onChangeAgentMemoryEnabled}
            onChangeAgentMemorySlots={onChangeAgentMemorySlots}
          />

          <Separator className="h-px" />

          <AgentRuntimeExperienceSection
            open={openSections.experience}
            homeTitle={homeTitle}
            inputPlaceholder={inputPlaceholder}
            suggestedQuestions={suggestedQuestions}
            isGeneratingSuggestions={isGeneratingSuggestions}
            systemPrompt={systemPrompt}
            modelValue={modelValue}
            defaultHomeTitle={defaultHomeTitle}
            defaultInputPlaceholder={defaultInputPlaceholder}
            onToggleSection={onToggleSection}
            onChangeHomeTitle={onChangeHomeTitle}
            onChangeInputPlaceholder={onChangeInputPlaceholder}
            onGenerateSuggestedQuestions={onGenerateSuggestedQuestions}
            onChangeSuggestedQuestions={onChangeSuggestedQuestions}
          />
        </div>
      </ScrollArea>
    </section>
  );
}
