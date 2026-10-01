'use client';

import { CheckCircle2, CircleSlash2, CircleHelp } from 'lucide-react';
import { useState } from 'react';
import { useLocale } from 'next-intl';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { resolveConnectorLocalizedText } from '../connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';
import type { ConnectorCapabilitySummary } from '@/services/types/connector';
import { capabilityGroup } from './capability-groups';

export function ConnectorCapabilityList({
  capabilities,
}: {
  capabilities: ConnectorCapabilitySummary[];
}) {
  const t = useT('connectors');
  const locale = useLocale();
  const [expanded, setExpanded] = useState(false);
  const [group, setGroup] = useState('all');
  const grouped = capabilities.length > 0 && capabilities.every(op => op.id.startsWith('feishu.'));
  const visible =
    grouped && group !== 'all'
      ? capabilities.filter(op => capabilityGroup(op.id) === group)
      : capabilities;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.capabilitiesTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        {grouped && (
          <div className="mb-4 flex flex-wrap items-center gap-2 text-sm">
            <Label htmlFor="connector-capability-group">{t('capabilityGroups.label')}</Label>
            <Select
              value={group}
              onValueChange={value => {
                setGroup(value);
                setExpanded(false);
              }}
            >
              <SelectTrigger id="connector-capability-group" className="w-full sm:w-60">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t('capabilityGroups.all')} ({capabilities.length})
                </SelectItem>
                {(['messaging', 'documents', 'people', 'resources'] as const).map(key => (
                  <SelectItem key={key} value={key}>
                    {t(`capabilityGroups.${key}`)} (
                    {capabilities.filter(op => capabilityGroup(op.id) === key).length})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <ul
          className="max-h-80 divide-y overflow-y-auto overscroll-contain pr-1"
          role="list"
          tabIndex={0}
          aria-label={t('detail.capabilitiesTitle')}
        >
          {(expanded ? visible : visible.slice(0, 5)).map(capability => (
            <li key={capability.id} className="flex items-start gap-3 py-4 first:pt-0 last:pb-0">
              {capability.availability === 'conditional' ||
              capability.availability === 'context_required' ? (
                <CircleHelp aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-warning" />
              ) : capability.usable ? (
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
                    <Badge variant="warning">{t('detail.checkAtUse')}</Badge>
                  )}
                  {capability.availability === 'context_required' && (
                    <Badge variant="subtle">{t('messaging.contextRequired')}</Badge>
                  )}
                </div>
                {capability.permission === 'unknown' && (
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t('messaging.permissionUnknown')}
                  </p>
                )}
              </div>
            </li>
          ))}
        </ul>
        {visible.length > 5 && (
          <Button
            variant="ghost"
            size="sm"
            className="mt-3"
            aria-expanded={expanded}
            onClick={() => setExpanded(!expanded)}
          >
            {t(expanded ? 'detail.showLess' : 'detail.showAll', { count: visible.length })}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
