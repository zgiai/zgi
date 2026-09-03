'use client';

import type { FormEvent } from 'react';
import { ExternalLink } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useT } from '@/i18n';
import type { ConnectorKeyField } from '@/services/types/connector';

interface ConnectorKeyFormProps {
  fields: ConnectorKeyField[];
  values: Record<string, string>;
  canSubmit: boolean;
  onValueChange: (name: string, value: string) => void;
  onSubmit: () => void;
  submitLabel?: string;
}

export function ConnectorKeyForm({
  fields,
  values,
  canSubmit,
  onValueChange,
  onSubmit,
  submitLabel,
}: ConnectorKeyFormProps) {
  const t = useT('connectors');
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (canSubmit) onSubmit();
  };

  return (
    <form className="space-y-4" onSubmit={submit}>
      {fields.map(field => {
        const fieldId = `connector-key-${field.name.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
        return (
          <div key={field.name} className="space-y-2">
            <Label htmlFor={fieldId}>{field.label || t('setup.fieldFallback')}</Label>
            <Input
              id={fieldId}
              name={field.name}
              type="password"
              value={values[field.name] ?? ''}
              placeholder={field.placeholder}
              required={field.required}
              autoComplete="new-password"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              onChange={event => onValueChange(field.name, event.target.value)}
            />
            {field.help_url ? (
              <a
                href={field.help_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {t('setup.helpLink')}
                <ExternalLink aria-hidden="true" className="size-3" />
              </a>
            ) : null}
          </div>
        );
      })}
      <Button type="submit" className="w-full" disabled={!canSubmit}>
        {submitLabel ?? t('setup.submit')}
      </Button>
    </form>
  );
}
