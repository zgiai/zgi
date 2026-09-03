'use client';

import { CheckCircle2, CircleSlash2 } from 'lucide-react';
import { useState } from 'react';
import { useLocale } from 'next-intl';
import { Button } from '@/components/ui/button';
import { resolveConnectorLocalizedText } from '../connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';
import type { ConnectorCapabilitySummary } from '@/services/types/connector';

export function ConnectorCapabilityList({
  capabilities,
}: {
  capabilities: ConnectorCapabilitySummary[];
}) {
  const t = useT('connectors');
  const locale = useLocale();
  const [expanded, setExpanded] = useState(false);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.capabilitiesTitle')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('detail.capabilitiesDescription')}</p>
      </CardHeader>
      <CardContent>
        <ul
          className="max-h-80 divide-y overflow-y-auto overscroll-contain pr-1"
          role="list"
          tabIndex={0}
          aria-label={t('detail.capabilitiesTitle')}
        >
          {(expanded ? capabilities : capabilities.slice(0, 5)).map(capability => (
            <li key={capability.id} className="flex items-start gap-3 py-4 first:pt-0 last:pb-0">
              {capability.usable ? (
                <CheckCircle2 aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-success" />
              ) : (
                <CircleSlash2
                  aria-hidden="true"
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                />
              )}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-sm font-medium">
                    {resolveConnectorLocalizedText(capability.name_i18n, locale, capability.name)}
                  </h3>
                  {!capability.usable ? (
                    <Badge variant="subtle">
                      {t(
                        capability.unavailable_reason === 'connector_not_verified'
                          ? 'app.notVerified'
                          : 'detail.capabilityUnavailable'
                      )}
                    </Badge>
                  ) : null}
                  {capability.availability === 'conditional' && (
                    <Badge variant="warning">{t('messaging.conditional')}</Badge>
                  )}
                  {capability.availability === 'context_required' && (
                    <Badge variant="subtle">{t('messaging.contextRequired')}</Badge>
                  )}
                </div>
                <p className="mt-1 text-sm leading-5 text-muted-foreground">
                  {resolveConnectorLocalizedText(
                    capability.description_i18n,
                    locale,
                    capability.description
                  )}
                </p>
                {capability.permission === 'unknown' && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('messaging.permissionUnknown')}
                  </p>
                )}
              </div>
            </li>
          ))}
        </ul>
        {capabilities.length > 5 && (
          <Button
            variant="ghost"
            size="sm"
            className="mt-3"
            aria-expanded={expanded}
            onClick={() => setExpanded(!expanded)}
          >
            {t(expanded ? 'detail.showLess' : 'detail.showAll', { count: capabilities.length })}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
