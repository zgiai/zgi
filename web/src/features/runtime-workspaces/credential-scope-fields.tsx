'use client';

import * as React from 'react';
import { Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Radio, RadioGroup } from '@/components/ui/radio';
import { RuntimeAppIcon } from '@/features/runtime-workspaces/app-presentation';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  RuntimeApp,
  RuntimeCredential,
  RuntimeCredentialScope,
} from '@/services/types/runtime-workspace';

const scopes: RuntimeCredentialScope[] = ['runtime', 'end_users.read', 'end_users.manage'];
// The message key of each scope; scopes contain dots, which keys may not.
const scopeKeys = {
  runtime: 'runtime',
  'end_users.read': 'endUsersRead',
  'end_users.manage': 'endUsersManage',
} as const;

export interface CredentialLimits {
  limited: boolean;
  scopes: RuntimeCredentialScope[];
  someApps: boolean;
  appKeys: string[];
}

export const fullCredentialLimits: CredentialLimits = {
  limited: false,
  scopes: ['runtime'],
  someApps: false,
  appKeys: [],
};

/** Whether the app choice applies: the key runs apps. */
function runsApps(limits: CredentialLimits) {
  return !limits.limited || limits.scopes.includes('runtime');
}

/** The scopes and apps to send, or null while the choice is incomplete. */
export function credentialLimitsInput(
  limits: CredentialLimits
): { scopes?: RuntimeCredentialScope[]; app_keys?: string[] } | null {
  if (limits.limited && limits.scopes.length === 0) return null;
  const someApps = runsApps(limits) && limits.someApps;
  if (someApps && limits.appKeys.length === 0) return null;
  return {
    ...(limits.limited ? { scopes: limits.scopes } : {}),
    ...(someApps ? { app_keys: limits.appKeys } : {}),
  };
}

/**
 * @component CredentialScopeFields
 * @category Feature
 * @status Beta
 * @description Chooses what a new access key may call (ADR 0012): full
 * access or some scopes, and every app or some apps. Scopes are fixed once
 * the key is created.
 * @usage Rendered in the create access key dialog.
 * @example
 * <CredentialScopeFields apps={apps} value={limits} onChange={setLimits} />
 */
export function CredentialScopeFields({
  apps,
  value,
  onChange,
}: {
  apps: RuntimeApp[];
  value: CredentialLimits;
  onChange: (value: CredentialLimits) => void;
}) {
  const t = useT('workspace');
  const [search, setSearch] = React.useState('');
  const q = search.trim().toLowerCase();
  const visible = apps.filter(
    app => !q || app.name.toLowerCase().includes(q) || app.app_key.toLowerCase().includes(q)
  );
  const toggleScope = (scope: RuntimeCredentialScope) =>
    onChange({
      ...value,
      scopes: value.scopes.includes(scope)
        ? value.scopes.filter(item => item !== scope)
        : [...value.scopes, scope],
    });
  const toggleApp = (appKey: string) =>
    onChange({
      ...value,
      appKeys: value.appKeys.includes(appKey)
        ? value.appKeys.filter(item => item !== appKey)
        : [...value.appKeys, appKey],
    });

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t('runtime.credentials.access.title')}</Label>
        <RadioGroup
          value={value.limited ? 'limited' : 'full'}
          onValueChange={mode => onChange({ ...value, limited: mode === 'limited' })}
          className="grid gap-2 sm:grid-cols-2"
        >
          {(['full', 'limited'] as const).map(mode => (
            <div
              key={mode}
              className={cn(
                'rounded-lg border p-3',
                (mode === 'limited') === value.limited && 'border-primary'
              )}
            >
              <Radio
                value={mode}
                label={t(`runtime.credentials.access.${mode}`)}
                description={t(`runtime.credentials.access.${mode}Hint`)}
              />
            </div>
          ))}
        </RadioGroup>
        {value.limited ? (
          <div className="space-y-2 rounded-lg border p-3">
            {scopes.map(scope => (
              <label key={scope} className="flex cursor-pointer items-start gap-2">
                <Checkbox
                  checked={value.scopes.includes(scope)}
                  onCheckedChange={() => toggleScope(scope)}
                  className="mt-0.5"
                />
                <span className="space-y-0.5">
                  <span className="block text-sm">
                    {t(`runtime.credentials.access.scopes.${scopeKeys[scope]}`)}
                  </span>
                  <span className="block text-xs text-muted-foreground">
                    {t(`runtime.credentials.access.scopeHints.${scopeKeys[scope]}`)}
                  </span>
                </span>
              </label>
            ))}
            {value.scopes.length === 0 ? (
              <p className="text-xs text-warning">{t('runtime.credentials.access.noScope')}</p>
            ) : null}
          </div>
        ) : null}
      </div>

      {runsApps(value) ? (
        <div className="space-y-2">
          <Label>{t('runtime.credentials.access.apps')}</Label>
          <RadioGroup
            value={value.someApps ? 'some' : 'all'}
            onValueChange={mode => onChange({ ...value, someApps: mode === 'some' })}
            orientation="horizontal"
            className="flex flex-wrap gap-4"
          >
            {(['all', 'some'] as const).map(mode => (
              <Radio key={mode} value={mode} label={t(`runtime.credentials.access.${mode}Apps`)} />
            ))}
          </RadioGroup>
          {value.someApps ? (
            <div className="space-y-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={event => setSearch(event.target.value)}
                  placeholder={t('runtime.credentials.access.searchApps')}
                  className="h-9 pl-9"
                />
              </div>
              <div className="max-h-48 divide-y overflow-y-auto rounded-lg border">
                {visible.map(app => (
                  <label
                    key={app.app_key}
                    className="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-muted/30"
                  >
                    <Checkbox
                      checked={value.appKeys.includes(app.app_key)}
                      onCheckedChange={() => toggleApp(app.app_key)}
                    />
                    <RuntimeAppIcon app={app} />
                    <span className="min-w-0 flex-1 truncate text-sm">{app.name}</span>
                    <code className="shrink-0 text-[11px] text-muted-foreground">
                      {app.app_key}
                    </code>
                  </label>
                ))}
              </div>
              <p
                className={
                  value.appKeys.length === 0
                    ? 'text-xs text-warning'
                    : 'text-xs text-muted-foreground'
                }
              >
                {value.appKeys.length === 0
                  ? t('runtime.credentials.access.noApps')
                  : t('runtime.credentials.access.appsSelected', { count: value.appKeys.length })}
              </p>
            </div>
          ) : null}
        </div>
      ) : null}
      <p className="text-xs text-muted-foreground">{t('runtime.credentials.access.fixed')}</p>
    </div>
  );
}

/**
 * @component CredentialScopeBadges
 * @category Feature
 * @status Beta
 * @description What an access key may call, as badges.
 * @usage Rendered in the access key table.
 * @example
 * <CredentialScopeBadges credential={credential} />
 */
export function CredentialScopeBadges({ credential }: { credential: RuntimeCredential }) {
  const t = useT('workspace');
  const credentialScopes = credential.scopes ?? [];
  const appKeys = credential.app_keys ?? [];
  const limited = credentialScopes.length > 0;
  const apps = appKeys.length;
  return (
    <div className="flex flex-wrap gap-1">
      {limited ? (
        credentialScopes.map(scope => (
          <Badge key={scope} variant="subtle" className="font-normal">
            {t(`runtime.credentials.access.scopes.${scopeKeys[scope]}`)}
          </Badge>
        ))
      ) : (
        <Badge variant="outline" className="font-normal">
          {t('runtime.credentials.access.full')}
        </Badge>
      )}
      {apps > 0 ? (
        <Badge variant="outline" className="font-normal" title={appKeys.join(', ')}>
          {t('runtime.credentials.access.appsCount', { count: apps })}
        </Badge>
      ) : null}
    </div>
  );
}
