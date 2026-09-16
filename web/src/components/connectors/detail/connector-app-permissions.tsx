'use client';

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ChevronRight } from 'lucide-react';

import { Button } from '@/components/ui/button';

import { connectorPermissionRows } from './permission-evidence-model';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type {
  ConnectorCatalogItem,
  ConnectorPermissionTemplate,
  ConnectorProviderEvidence,
} from '@/services/types/connector';

const permissionList = (items: string[]) => (
  <ul className="mt-3 space-y-2">
    {items.map(item => (
      <li className="break-all font-mono text-xs text-muted-foreground" key={item}>
        {item}
      </li>
    ))}
  </ul>
);

// Shared by the existing setup dialog and connection details. This is a
// configuration preview, never an authorization result or editable scope list.
export function ConnectorPermissionTemplateDetails({
  template,
}: {
  template?: ConnectorPermissionTemplate;
}) {
  const t = useT('connectors');
  if (!template?.addons) return null;
  const tenantScopes = template.addons.scopes?.tenant ?? [];
  const userScopes = template.addons.scopes?.user ?? [];
  const tenantEvents = template.addons.events?.items?.tenant ?? [];
  const userEvents = template.addons.events?.items?.user ?? [];
  const callbacks = template.addons.callbacks?.items ?? [];
  const dependencies = template.user_event_dependencies ?? [];
  return (
    <Collapsible className="rounded-lg border p-3">
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-sm"
        >
          <ChevronRight
            aria-hidden="true"
            className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
          />
          <span className="min-w-0">
            {t('app.template')}
            {template.profile_version && (
              <span className="ml-2 font-mono text-xs text-muted-foreground">
                {template.profile_version}
              </span>
            )}
          </span>
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3">
        <div className="mt-3 max-h-80 space-y-4 overflow-y-auto pr-2">
          <p className="text-xs leading-5 text-muted-foreground">{t('app.configurationOnly')}</p>
          <p className="text-xs leading-5 text-muted-foreground">
            {t('app.templateIncrementalHint')}
          </p>
          <div>
            <h4 className="text-sm font-medium">
              {t('app.appScopes')} · {tenantScopes.length}
            </h4>
            {permissionList(tenantScopes)}
          </div>
          <div>
            <h4 className="text-sm font-medium">
              {t('app.userScopes')} · {userScopes.length}
            </h4>
            {permissionList(userScopes)}
          </div>
          <div className="border-t pt-3">
            <h4 className="text-sm font-medium">{t('app.events')}</h4>
            <p className="mt-2 text-xs text-muted-foreground">
              {t('app.appEvents')} · {tenantEvents.length}
            </p>
            {permissionList(tenantEvents)}
            <p className="mt-3 text-xs text-muted-foreground">
              {t('app.userEvents')} · {userEvents.length}
            </p>
            {permissionList(userEvents)}
            {dependencies.length > 0 && (
              <div className="mt-3 rounded-md border bg-muted/20 p-3">
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('app.userEventDependencyHint')}
                </p>
                <ul className="mt-3 space-y-3">
                  {dependencies.map(dependency => (
                    <li key={dependency.event_id} className="space-y-1 break-all text-xs">
                      <p className="font-mono">{dependency.event_id}</p>
                      <p>
                        {t('app.requiredUserScope')}: <code>{dependency.user_scope}</code>
                      </p>
                      <p>
                        {t('app.requiredSubscription')}: <code>{dependency.subscription_api}</code>
                      </p>
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <p className="mt-3 text-xs text-muted-foreground">
              {t('app.callbacks')} · {callbacks.length}
            </p>
            {permissionList(callbacks)}
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export function ConnectorAppPermissions({
  connector,
  evidence,
}: {
  connector: ConnectorCatalogItem;
  evidence?: ConnectorProviderEvidence;
}) {
  const t = useT('connectors');
  const rows = connectorPermissionRows(connector.permission_template, evidence);
  return (
    <Collapsible className="rounded-xl border bg-background p-5">
      <CollapsibleTrigger asChild>
        <Button
          variant="ghost"
          className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-sm"
        >
          <ChevronRight
            aria-hidden="true"
            className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
          />
          <span className="min-w-0">{t('app.permissionDetails')}</span>
        </Button>
      </CollapsibleTrigger>
      <p className="mt-2 text-xs text-muted-foreground">
        {t('app.permissionSummary', {
          verified: rows.filter(row => row.status === 'verified').length,
          missing: rows.filter(row => row.status === 'not_granted').length,
          unknown: rows.filter(row => row.status === 'unverified').length,
        })}
      </p>
      <CollapsibleContent className="space-y-3">
        <div className="mt-4 space-y-4 text-sm">
          {evidence ? (
            <div>
              <p className="font-medium">{evidence.app_name}</p>
              <p className="mt-1 break-all font-mono text-xs">{evidence.app_id}</p>
              <p className="text-muted-foreground">
                {t(evidence.region === 'lark' ? 'app.lark' : 'app.feishu')}
              </p>
            </div>
          ) : (
            <Badge variant="subtle">{t('app.notVerified')}</Badge>
          )}
          <p className="text-xs text-muted-foreground">
            {t('app.configurationOnly')} {t('app.scopeHint')}
          </p>
          <ul className="max-h-80 divide-y overflow-y-auto">
            {rows.map(row => (
              <li
                key={row.identity + ':' + row.scope}
                className="flex items-start justify-between gap-4 py-2"
              >
                <span className="min-w-0">
                  <code className="break-all text-xs">{row.scope}</code>
                  <span className="block text-xs text-muted-foreground">
                    {t(row.identity === 'tenant' ? 'app.appScopes' : 'app.userScopes')}
                  </span>
                </span>
                <Badge variant="subtle" className="shrink-0">
                  {t(
                    row.status === 'verified'
                      ? 'app.permissionVerified'
                      : row.status === 'not_granted'
                        ? 'app.permissionNotGranted'
                        : 'app.unknown'
                  )}
                </Badge>
              </li>
            ))}
          </ul>
          <p className="text-xs text-muted-foreground">
            {t('app.template')} ·{' '}
            {connector.permission_template?.profile_version ?? t('app.unknown')}
          </p>
          <p className="text-xs text-muted-foreground">{t('app.templateIncrementalHint')}</p>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
