'use client';

import { TriangleAlert } from 'lucide-react';
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
  recommended?: boolean;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
}

export function AgentRuntimeModelSection({
  open,
  modelValue,
  unavailable = false,
  recommended = true,
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
        preferredUseCase="agent"
        capabilityFilter={{ features_tool_call: true }}
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
      {!unavailable && !recommended ? (
        <div className="mt-2 flex gap-2 rounded-md border border-warning/20 bg-warning/10 p-2 text-xs leading-5 text-warning-foreground">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          <span>{t('modelSelection.compatibilityWarning')}</span>
        </div>
      ) : null}
    </RuntimeSection>
  );
}
