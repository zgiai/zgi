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
  unavailable?: boolean;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
}

export function AgentRuntimeModelSection({
  open,
  modelValue,
  unavailable = false,
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
        modelType="agent"
        value={modelValue}
        onChange={onChangeModelValue}
        hasError={unavailable}
        disabled={readOnly}
        className="w-full"
      />
      {unavailable ? (
        <div className="mt-2 rounded-md border border-destructive/20 bg-destructive/5 p-2 text-xs text-destructive">
          {t('toasts.modelUnavailable')}
        </div>
      ) : null}
    </RuntimeSection>
  );
}
