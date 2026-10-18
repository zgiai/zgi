'use client';

import { Checkbox } from '@/components/ui/checkbox';
import { useT } from '@/i18n';
import type { FileUploadSelection } from '@/services/upload.service';

export function FileUploadTypeSelector({
  value,
  onChange,
  disabled = false,
}: {
  value?: FileUploadSelection | null;
  onChange: (value: FileUploadSelection) => void;
  disabled?: boolean;
}) {
  const t = useT('webapp');
  const types = value?.allowed_file_types ?? ['document', 'image'];
  return (
    <fieldset disabled={disabled} className="space-y-2">
      <legend className="mb-2 text-sm font-medium">
        {t('consoleChat.attachments.allowedTypes')}
      </legend>
      <div className="flex gap-6">
        {(['document', 'image'] as const).map(type => (
          <label key={type} className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={types.includes(type)}
              disabled={disabled}
              onCheckedChange={checked =>
                onChange({
                  allowed_file_types: (['document', 'image'] as const).filter(item =>
                    item === type ? checked === true : types.includes(item)
                  ),
                })
              }
            />
            {t(
              type === 'document'
                ? 'consoleChat.attachments.documentType'
                : 'consoleChat.attachments.imageType'
            )}
          </label>
        ))}
      </div>
    </fieldset>
  );
}
