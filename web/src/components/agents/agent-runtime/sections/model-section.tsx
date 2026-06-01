'use client';

import {
  ModelSelectorParameter,
  type ModelSelectorParameterValue,
} from '@/components/common/model-selector';
import { useT } from '@/i18n';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeModelSectionProps {
  open: boolean;
  modelValue: ModelSelectorParameterValue;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
}

export function AgentRuntimeModelSection({
  open,
  modelValue,
  onToggleSection,
  onChangeModelValue,
}: AgentRuntimeModelSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <RuntimeSection
      title={t('sections.model')}
      section="model"
      open={open}
      onToggle={onToggleSection}
    >
      <ModelSelectorParameter
        modelType="text-chat"
        value={modelValue}
        onChange={onChangeModelValue}
        className="w-full"
      />
    </RuntimeSection>
  );
}
