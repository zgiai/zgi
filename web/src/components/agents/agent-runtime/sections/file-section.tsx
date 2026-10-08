'use client';

import { Switch } from '@/components/ui/switch';
import { FileUploadTypeSelector } from '@/components/chat/file-upload-type-selector';
import type { FileUploadSelection } from '@/services/upload.service';
import { useT } from '@/i18n';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeFileSectionProps {
  open: boolean;
  fileUploadEnabled: boolean;
  fileUploadPolicy?: FileUploadSelection | null;
  onChangeFileUploadPolicy: (value: FileUploadSelection) => void;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
}

export function AgentRuntimeFileSection({
  open,
  fileUploadEnabled,
  fileUploadPolicy,
  onChangeFileUploadPolicy,
  readOnly = false,
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
        <Switch
          checked={fileUploadEnabled}
          disabled={readOnly}
          onCheckedChange={onChangeFileUploadEnabled}
        />
      </div>
      {fileUploadEnabled ? <div className="mt-3"><FileUploadTypeSelector value={fileUploadPolicy} onChange={onChangeFileUploadPolicy} disabled={readOnly} /></div> : null}
    </RuntimeSection>
  );
}
