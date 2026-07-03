'use client';

import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeFileSectionProps {
  open: boolean;
  fileUploadEnabled: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
}

export function AgentRuntimeFileSection({
  open,
  fileUploadEnabled,
  onToggleSection,
  onChangeFileUploadEnabled,
}: AgentRuntimeFileSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <RuntimeSection
      title={t('sections.files')}
      section="files"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="flex items-center justify-between rounded-md border p-3">
        <div>
          <div className="text-sm font-medium">{t('files.title')}</div>
          <div className="text-xs text-muted-foreground">{t('files.description')}</div>
        </div>
        <Switch checked={fileUploadEnabled} onCheckedChange={onChangeFileUploadEnabled} />
      </div>
    </RuntimeSection>
  );
}
