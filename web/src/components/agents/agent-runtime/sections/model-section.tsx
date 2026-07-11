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
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
}

export function AgentRuntimeModelSection({
  open,
  modelValue,
  readOnly = false,
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
        disabled={readOnly}
        className="w-full"
      />
    </RuntimeSection>
  );
}
