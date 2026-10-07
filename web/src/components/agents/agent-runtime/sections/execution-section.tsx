'use client';

import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import type { AgentExecutionRuntimeConfig } from '@/services/types/agent';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeExecutionSectionProps {
  open: boolean;
  value: AgentExecutionRuntimeConfig | null;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChange: (value: AgentExecutionRuntimeConfig) => void;
}

export function AgentRuntimeExecutionSection({
  open,
  value,
  readOnly = false,
  onToggleSection,
  onChange,
}: AgentRuntimeExecutionSectionProps) {
  const t = useT('agents.agentRuntime');
  const config = value ?? {
    enabled: false,
  };

  return (
    <RuntimeSection
      title={t('sections.execution')}
      section="execution"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="space-y-3 rounded-md border p-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="text-sm font-medium">{t('execution.title')}</div>
            <p className="text-xs text-muted-foreground">{t('execution.description')}</p>
          </div>
          <Switch
            checked={config.enabled}
            disabled={readOnly}
            aria-label={t('execution.title')}
            onCheckedChange={enabled => onChange({ ...config, enabled })}
          />
        </div>
        <div className="space-y-1 text-xs text-muted-foreground">
          <p>{config.enabled ? t('execution.enabledSummary') : t('execution.disabledSummary')}</p>
          <p>{t('execution.platformManagedSummary')}</p>
          <p>{t('execution.lifecycleSummary')}</p>
        </div>
      </div>
    </RuntimeSection>
  );
}
