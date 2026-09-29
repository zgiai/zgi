'use client';

import { Input } from '@/components/ui/input';
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
    profile_id: '',
    policy_id: '',
    home_snapshot_id: null,
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
        {config.enabled ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="space-y-1 text-xs font-medium">
              <span>{t('execution.profileId')}</span>
              <Input
                value={config.profile_id}
                disabled={readOnly}
                maxLength={128}
                placeholder="standard"
                onChange={event => onChange({ ...config, profile_id: event.target.value })}
              />
            </label>
            <label className="space-y-1 text-xs font-medium">
              <span>{t('execution.policyId')}</span>
              <Input
                value={config.policy_id}
                disabled={readOnly}
                maxLength={128}
                placeholder="offline"
                onChange={event => onChange({ ...config, policy_id: event.target.value })}
              />
            </label>
            <p className="text-xs text-muted-foreground sm:col-span-2">
              {t('execution.approvalNotice')}
            </p>
          </div>
        ) : null}
      </div>
    </RuntimeSection>
  );
}
