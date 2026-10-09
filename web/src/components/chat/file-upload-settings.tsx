'use client';

import { useRef } from 'react';
import { Switch } from '@/components/ui/switch';
import { FileUploadTypeSelector } from './file-upload-type-selector';
import type { FileUploadSelection } from '@/services/upload.service';
import { useT } from '@/i18n';

export function fileUploadSelectionEnabled(value?: FileUploadSelection | null) {
  return value == null || value.allowed_file_types.length > 0;
}

export function FileUploadSettings({
  enabled,
  value,
  onChangeEnabled,
  onChange,
  disabled = false,
}: {
  enabled: boolean;
  value?: FileUploadSelection | null;
  onChangeEnabled: (enabled: boolean) => void;
  onChange: (value: FileUploadSelection) => void;
  disabled?: boolean;
}) {
  const t = useT('agents.agentRuntime');
  return (
    <>
      <div className="flex items-center justify-between gap-3 rounded-md border p-3">
        <div>
          <div className="text-sm font-medium">{t('files.title')}</div>
          <div className="text-xs text-muted-foreground">{t('files.description')}</div>
        </div>
        <Switch
          aria-label={t('files.title')}
          checked={enabled}
          disabled={disabled}
          onCheckedChange={onChangeEnabled}
        />
      </div>
      {enabled ? (
        <div className="mt-3">
          <FileUploadTypeSelector value={value} onChange={onChange} disabled={disabled} />
        </div>
      ) : null}
    </>
  );
}

// DigitalHuman already persists an empty type list as upload-disabled. Keep
// that single source of truth instead of adding a second enablement field.
export function PolicyFileUploadSettings({
  value,
  onChange,
  disabled = false,
}: {
  value?: FileUploadSelection | null;
  onChange: (value: FileUploadSelection) => void;
  disabled?: boolean;
}) {
  const enabled = fileUploadSelectionEnabled(value);
  const lastEnabled = useRef<FileUploadSelection>(
    value && enabled ? value : { allowed_file_types: ['document', 'image'] }
  );
  return (
    <FileUploadSettings
      enabled={enabled}
      value={value}
      disabled={disabled}
      onChange={next => {
        if (fileUploadSelectionEnabled(next)) lastEnabled.current = next;
        onChange(next);
      }}
      onChangeEnabled={next => {
        if (!next && enabled && value) lastEnabled.current = value;
        onChange(next ? lastEnabled.current : { allowed_file_types: [] });
      }}
    />
  );
}
