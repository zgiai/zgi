'use client';

import { FileUploadSettings } from '@/components/chat/file-upload-settings';
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
      <FileUploadSettings
        enabled={fileUploadEnabled}
        value={fileUploadPolicy}
        onChangeEnabled={onChangeFileUploadEnabled}
        onChange={onChangeFileUploadPolicy}
        disabled={readOnly}
      />
    </RuntimeSection>
  );
}
