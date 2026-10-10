'use client';

import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import type {
  AgentExecutionRuntimeCapability,
  AgentExecutionRuntimeConfig,
} from '@/services/types/agent';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeExecutionSectionProps {
  open: boolean;
  value: AgentExecutionRuntimeConfig | null;
  capability?: AgentExecutionRuntimeCapability | null;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChange: (value: AgentExecutionRuntimeConfig) => void;
}

export function AgentRuntimeExecutionSection({
  open,
  value,
  capability,
  readOnly = false,
  onToggleSection,
  onChange,
}: AgentRuntimeExecutionSectionProps) {
  const t = useT('agents.agentRuntime');
  const config = value ?? {
    enabled: false,
  };
  const canEnable = capability?.can_enable ?? true;
  const unavailable = capability != null && !capability.available && !config.enabled;

  return (
    <RuntimeSection
      title={t('sections.execution')}
      section="execution"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="flex items-center justify-between gap-3 rounded-md border p-3">
        <div>
          <div className="text-sm font-medium">{t('execution.title')}</div>
          <p className="text-xs text-muted-foreground">{t('execution.description')}</p>
          {unavailable ? (
            <p className="mt-1 text-xs text-muted-foreground">{t('execution.unavailable')}</p>
          ) : null}
        </div>
        <Switch
          checked={config.enabled}
          disabled={readOnly || (!config.enabled && !canEnable)}
          aria-label={t('execution.title')}
          onCheckedChange={enabled => onChange({ enabled })}
        />
      </div>
    </RuntimeSection>
  );
}
