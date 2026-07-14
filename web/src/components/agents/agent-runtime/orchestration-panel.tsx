'use client';

import type { ModelSelectorParameterValue } from '@/components/common/model-selector';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { OpeningGuideBrand } from '@/components/chat/utils/opening-guide-brand';
import type { OpeningStatementDialogValue } from '@/components/workflow/ui/features-panel/opening-statement-dialog';
import type {
  AgentDatabaseBinding,
  AgentMemorySlotConfig,
  AgentWorkflowBinding,
  AgentWorkflowBindingCandidate,
} from '@/services/types/agent';
import type { Dataset } from '@/services/types/dataset';
import { AgentRuntimeDatabaseSection } from './sections/database-section';
import { AgentRuntimeExperienceSection } from './sections/experience-section';
import { AgentRuntimeFileSection } from './sections/file-section';
import { AgentRuntimeKnowledgeSection } from './sections/knowledge-section';
import { AgentRuntimeMemorySection } from './sections/memory-section';
import { AgentRuntimeModelSection } from './sections/model-section';
import { AgentRuntimeSkillSection } from './sections/skill-section';
import { AgentRuntimeWorkflowSection } from './sections/workflow-section';
import type { AgentConfigSection } from './types';
import type { AgentMemorySlotValidationError } from './utils';

interface AgentRuntimeOrchestrationPanelProps {
  locale: string;
  agentWorkspaceId?: string;
  openSections: Record<AgentConfigSection, boolean>;
  modelValue: ModelSelectorParameterValue;
  isAgentModelUnavailable: boolean;
  homeTitle: string;
  openingStatement: string;
  inputPlaceholder: string;
  selectedSkills: AIChatSkillMetadata[];
  normalizedSelectedSkillIds: string[];
  selectableSkillsCount: number;
  isSkillsLoading: boolean;
  isSkillConfigLoading: boolean;
  isDatasetsLoading: boolean;
  canBindKnowledge: boolean;
  selectedKnowledgeDatasets: Dataset[];
  selectedKnowledgeDatasetIds: string[];
  databaseBindings: AgentDatabaseBinding[];
  workflowBindings: AgentWorkflowBinding[];
  workflowCandidatesByBindingID: Map<string, AgentWorkflowBindingCandidate>;
  isWorkflowCandidatesLoading: boolean;
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  fileUploadEnabled: boolean;
  agentMemoryEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  openingGuideBrand?: OpeningGuideBrand;
  className?: string;
  scrollAreaClassName?: string;
  scrollViewportClassName?: string;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeOpeningStatement: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onOpenSkillDialog: () => void;
  onOpenKnowledgeDialog: () => void;
  onOpenWorkflowDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
  onChangeDatabaseBindings: (value: AgentDatabaseBinding[]) => void;
  onChangeWorkflowBindings: (value: AgentWorkflowBinding[]) => void;
  onGenerateSuggestedQuestions: (
    value: OpeningStatementDialogValue
  ) => Promise<{ questions: string[]; warnings?: string[] } | undefined>;
  onChangeSuggestedQuestions: (value: string[]) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
}

export function AgentRuntimeOrchestrationPanel({
  locale,
  agentWorkspaceId,
  openSections,
  modelValue,
  isAgentModelUnavailable,
  homeTitle,
  openingStatement,
  inputPlaceholder,
  selectedSkills,
  normalizedSelectedSkillIds,
  selectableSkillsCount,
  isSkillsLoading,
  isSkillConfigLoading,
  isDatasetsLoading,
  canBindKnowledge,
  selectedKnowledgeDatasets,
  selectedKnowledgeDatasetIds,
  databaseBindings,
  workflowBindings,
  workflowCandidatesByBindingID,
  isWorkflowCandidatesLoading,
  suggestedQuestions,
  isGeneratingSuggestions,
  fileUploadEnabled,
  agentMemoryEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  defaultHomeTitle,
  defaultInputPlaceholder,
  openingGuideBrand,
  className,
  scrollAreaClassName,
  scrollViewportClassName,
  readOnly = false,
  onToggleSection,
  onChangeModelValue,
  onChangeHomeTitle,
  onChangeOpeningStatement,
  onChangeInputPlaceholder,
  onOpenSkillDialog,
  onOpenKnowledgeDialog,
  onOpenWorkflowDialog,
  onToggleSkill,
  onToggleKnowledgeDataset,
  onChangeDatabaseBindings,
  onChangeWorkflowBindings,
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
            unavailable={isAgentModelUnavailable}
            readOnly={readOnly}
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
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onOpenSkillDialog={onOpenSkillDialog}
            onToggleSkill={onToggleSkill}
          />

          <Separator className="h-px" />

          <AgentRuntimeKnowledgeSection
            open={openSections.knowledge}
            isDatasetsLoading={isDatasetsLoading}
            canBindKnowledge={canBindKnowledge}
            selectedKnowledgeDatasets={selectedKnowledgeDatasets}
            selectedKnowledgeDatasetIds={selectedKnowledgeDatasetIds}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onOpenKnowledgeDialog={onOpenKnowledgeDialog}
            onToggleKnowledgeDataset={onToggleKnowledgeDataset}
          />

          <Separator className="h-px" />

          <AgentRuntimeDatabaseSection
            open={openSections.databases}
            workspaceId={agentWorkspaceId}
            bindings={databaseBindings}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onChangeBindings={onChangeDatabaseBindings}
          />

          <Separator className="h-px" />

          <AgentRuntimeWorkflowSection
            open={openSections.workflows}
            bindings={workflowBindings}
            candidatesByBindingID={workflowCandidatesByBindingID}
            isLoading={isWorkflowCandidatesLoading}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onOpenWorkflowDialog={onOpenWorkflowDialog}
            onChangeBindings={onChangeWorkflowBindings}
          />

          <Separator className="h-px" />

          <AgentRuntimeFileSection
            open={openSections.files}
            fileUploadEnabled={fileUploadEnabled}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onChangeFileUploadEnabled={onChangeFileUploadEnabled}
          />

          <Separator className="h-px" />

          <AgentRuntimeMemorySection
            open={openSections.memory}
            agentMemoryEnabled={agentMemoryEnabled}
            agentMemorySlots={agentMemorySlots}
            agentMemorySlotValidationErrors={agentMemorySlotValidationErrors}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onChangeAgentMemoryEnabled={onChangeAgentMemoryEnabled}
            onChangeAgentMemorySlots={onChangeAgentMemorySlots}
          />

          <Separator className="h-px" />

          <AgentRuntimeExperienceSection
            open={openSections.experience}
            homeTitle={homeTitle}
            openingStatement={openingStatement}
            inputPlaceholder={inputPlaceholder}
            suggestedQuestions={suggestedQuestions}
            isGeneratingSuggestions={isGeneratingSuggestions}
            defaultHomeTitle={defaultHomeTitle}
            defaultInputPlaceholder={defaultInputPlaceholder}
            openingGuideBrand={openingGuideBrand}
            readOnly={readOnly}
            onToggleSection={onToggleSection}
            onChangeHomeTitle={onChangeHomeTitle}
            onChangeOpeningStatement={onChangeOpeningStatement}
            onChangeInputPlaceholder={onChangeInputPlaceholder}
            onGenerateSuggestedQuestions={onGenerateSuggestedQuestions}
            onChangeSuggestedQuestions={onChangeSuggestedQuestions}
          />
        </div>
      </ScrollArea>
    </section>
  );
}
