'use client';

import { ExternalLink, KeyRound } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';
import type { ConnectorKeyField } from '@/services/types/connector';

export function ConnectorKeyRequirements({ fields }: { fields: ConnectorKeyField[] }) {
  const t = useT('connectors');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.requirementsTitle')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('detail.requirementsDescription')}</p>
      </CardHeader>
      <CardContent>
        <ul className="space-y-3" role="list">
          {fields.map(field => (
            <li key={field.name} className="flex items-start gap-3 rounded-lg bg-muted/30 p-3">
              <KeyRound
                aria-hidden="true"
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{field.label}</p>
                {field.help_url ? (
                  <a
                    href={field.help_url}
                    target="_blank"
                    rel="noreferrer"
                    className="mt-1 inline-flex items-center gap-1 text-xs text-primary hover:underline"
                  >
                    {t('setup.helpLink')}
                    <ExternalLink aria-hidden="true" className="size-3" />
                  </a>
                ) : null}
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
