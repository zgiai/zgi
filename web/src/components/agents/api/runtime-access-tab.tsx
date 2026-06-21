'use client';

import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { AppWindow, Globe2, KeyRound, Plus, Save, Users, Workflow } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  useAgentRuntimeSurfaces,
  useUpdateAgentRuntimeSurfaces,
} from '@/hooks/agent/use-agent-runtime-surfaces';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  AgentRuntimeGrantSubject,
  AgentRuntimeSurfaceAuthorization,
  UpdateAgentRuntimeSurfaceGrant,
} from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  RuntimeGrantSubjectRow,
  type RuntimeGrantSubjectLabels,
} from '@/components/runtime-auth/runtime-grant-subject-row';

const EDITABLE_AUDIENCE_SUBJECTS = ['organization', 'department', 'account'] as const;
const SOURCE_LABEL_KEYS: Record<
  string,
  'sources.legacy_agent_fields' | 'sources.grant' | 'sources.system_default'
> = {
  legacy_agent_fields: 'sources.legacy_agent_fields',
  grant: 'sources.grant',
  system_default: 'sources.system_default',
};

type EditableAudienceSubject = (typeof EDITABLE_AUDIENCE_SUBJECTS)[number];
type WebAppAudienceMode = 'public' | 'scoped';

interface RuntimeAccessTabProps {
  agentId: string;
  canManage: boolean;
}

interface EditableGrant {
  subject_type: EditableAudienceSubject;
  subject_id: string;
}

function defaultOrganizationGrant(): EditableGrant {
  return { subject_type: 'organization', subject_id: '' };
}

function isEditableAudienceSubject(
  subject: AgentRuntimeGrantSubject
): subject is EditableAudienceSubject {
  return EDITABLE_AUDIENCE_SUBJECTS.includes(subject as EditableAudienceSubject);
}

function normalizeSubjectId(grant: EditableGrant): string | null {
  return grant.subject_type === 'organization' ? null : grant.subject_id.trim();
}

function findSurface(
  surfaces: AgentRuntimeSurfaceAuthorization[] | undefined,
  name: string
): AgentRuntimeSurfaceAuthorization | null {
  return surfaces?.find(surface => surface.surface === name) ?? null;
}

function editableAudienceGrants(surface: AgentRuntimeSurfaceAuthorization | null): EditableGrant[] {
  const grants: EditableGrant[] = [];
  for (const grant of surface?.grants ?? []) {
    if (grant.enabled && isEditableAudienceSubject(grant.subject_type)) {
      grants.push({
        subject_type: grant.subject_type,
        subject_id: grant.subject_type === 'organization' ? '' : (grant.subject_id ?? ''),
      });
    }
  }
  return grants;
}

function surfaceHasPublicGrant(surface: AgentRuntimeSurfaceAuthorization | null): boolean {
  return Boolean(
    surface?.grants.some(grant => grant.enabled && grant.subject_type === 'public')
  );
}

export default function RuntimeAccessTab({ agentId, canManage }: RuntimeAccessTabProps) {
  const t = useT('agents.runtimeAccess');
  const [webAppEnabled, setWebAppEnabled] = useState(false);
  const [webAppAudienceMode, setWebAppAudienceMode] = useState<WebAppAudienceMode>('public');
  const [webAppGrants, setWebAppGrants] = useState<EditableGrant[]>([defaultOrganizationGrant()]);
  const [apiEnabled, setApiEnabled] = useState(false);
  const [builtinEnabled, setBuiltinEnabled] = useState(false);
  const [builtinGrants, setBuiltinGrants] = useState<EditableGrant[]>([defaultOrganizationGrant()]);

  const { data, error, isLoading, isFetching } = useAgentRuntimeSurfaces(agentId);
  const updateMutation = useUpdateAgentRuntimeSurfaces();
  const runtimeData = data?.data;

  const surfaces = runtimeData?.surfaces;
  const webAppSurface = useMemo(() => findSurface(surfaces, 'webapp'), [surfaces]);
  const apiSurface = useMemo(() => findSurface(surfaces, 'api'), [surfaces]);
  const builtinSurface = useMemo(() => findSurface(surfaces, 'builtin_app'), [surfaces]);
  const internalSurface = useMemo(() => findSurface(surfaces, 'internal'), [surfaces]);

  useEffect(() => {
    if (!runtimeData) return;

    setWebAppEnabled(webAppSurface?.enabled ?? false);
    setApiEnabled(apiSurface?.enabled ?? false);
    setBuiltinEnabled(builtinSurface?.enabled ?? false);

    const webAppEditableGrants = editableAudienceGrants(webAppSurface);
    const webAppHasEnabledGrants = Boolean(webAppSurface?.grants.some(grant => grant.enabled));
    setWebAppAudienceMode(
      surfaceHasPublicGrant(webAppSurface) || !webAppHasEnabledGrants ? 'public' : 'scoped'
    );
    setWebAppGrants(
      webAppEditableGrants.length > 0 ? webAppEditableGrants : [defaultOrganizationGrant()]
    );

    const builtinEditableGrants = editableAudienceGrants(builtinSurface);
    setBuiltinGrants(
      builtinEditableGrants.length > 0 ? builtinEditableGrants : [defaultOrganizationGrant()]
    );
  }, [apiSurface, builtinSurface, runtimeData, webAppSurface]);

  const updateWebAppGrant = (index: number, next: EditableGrant) => {
    setWebAppGrants(current => current.map((grant, i) => (i === index ? next : grant)));
  };

  const addWebAppGrant = () => {
    setWebAppGrants(current => [...current, { subject_type: 'department', subject_id: '' }]);
  };

  const removeWebAppGrant = (index: number) => {
    setWebAppGrants(current => current.filter((_, i) => i !== index));
  };

  const updateBuiltinGrant = (index: number, next: EditableGrant) => {
    setBuiltinGrants(current => current.map((grant, i) => (i === index ? next : grant)));
  };

  const addBuiltinGrant = () => {
    setBuiltinGrants(current => [...current, { subject_type: 'department', subject_id: '' }]);
  };

  const removeBuiltinGrant = (index: number) => {
    setBuiltinGrants(current => current.filter((_, i) => i !== index));
  };

  const buildEditableAudienceGrants = (
    grants: EditableGrant[],
    grantRequiredMessage: string
  ): UpdateAgentRuntimeSurfaceGrant[] | null => {
    const normalized = grants.map(grant => ({
      subject_type: grant.subject_type,
      subject_id: normalizeSubjectId(grant),
      enabled: true,
    }));
    if (
      normalized.some(grant => grant.subject_type !== 'organization' && !grant.subject_id?.trim())
    ) {
      toast.error(t('validation.subjectIdRequired'));
      return null;
    }

    const dedupeKeys = new Set<string>();
    for (const grant of normalized) {
      const key = `${grant.subject_type}:${grant.subject_id ?? ''}`;
      if (dedupeKeys.has(key)) {
        toast.error(t('validation.duplicateGrant'));
        return null;
      }
      dedupeKeys.add(key);
    }
    if (normalized.length === 0) {
      toast.error(grantRequiredMessage);
      return null;
    }
    return normalized;
  };

  const buildWebAppGrants = (): UpdateAgentRuntimeSurfaceGrant[] | null => {
    if (!webAppEnabled) return [];
    if (webAppAudienceMode === 'public') {
      return [{ subject_type: 'public', enabled: true }];
    }
    return buildEditableAudienceGrants(webAppGrants, t('validation.webappGrantRequired'));
  };

  const buildBuiltinGrants = (): UpdateAgentRuntimeSurfaceGrant[] | null => {
    if (!builtinEnabled) return [];
    return buildEditableAudienceGrants(builtinGrants, t('validation.grantRequired'));
  };

  const handleSave = async () => {
    if (!canManage) {
      toast.error(t('validation.manageRequired'));
      return;
    }
    const webApp = buildWebAppGrants();
    if (webApp === null) return;
    const builtin = buildBuiltinGrants();
    if (builtin === null) return;

    await updateMutation.mutateAsync({
      agentId,
      payload: {
        surfaces: [
          {
            surface: 'webapp',
            enabled: webAppEnabled,
            grants: webApp,
          },
          {
            surface: 'api',
            enabled: apiEnabled,
            grants: [{ subject_type: 'public', enabled: apiEnabled }],
          },
          {
            surface: 'builtin_app',
            enabled: builtinEnabled,
            grants: builtin,
          },
          {
            surface: 'internal',
            enabled: true,
            grants: [{ subject_type: 'internal', enabled: true }],
          },
        ],
      },
    });
  };

  const renderStatusBadge = (enabled: boolean) => (
    <Badge variant={enabled ? 'success' : 'subtle'}>
      {enabled ? t('status.enabled') : t('status.disabled')}
    </Badge>
  );

  const renderSourceBadge = (source?: string) =>
    source ? (
      <Badge variant="outline">{t(SOURCE_LABEL_KEYS[source] ?? 'sources.grant')}</Badge>
    ) : null;
  const grantSubjectLabels: RuntimeGrantSubjectLabels = {
    subjectLabels: {
      organization: t('grantSubjects.organization'),
      department: t('grantSubjects.department'),
      account: t('grantSubjects.account'),
    },
    organizationWide: t('grants.organizationWide'),
    departmentPlaceholder: t('grants.departmentPlaceholder'),
    accountPlaceholder: t('grants.accountPlaceholder'),
    searchMembersPlaceholder: t('grants.searchMembersPlaceholder'),
    noMembers: t('grants.noMembers'),
    loadingMembers: t('grants.loadingMembers'),
    resolvingAccount: t('grants.resolvingAccount'),
    selectionRequired: t('grants.selectionRequired'),
    accountLookupFailed: t('grants.accountLookupFailed'),
    departmentLookupFailed: t('grants.departmentLookupFailed'),
    unresolvedAccount: t('grants.unresolvedAccount'),
    unresolvedDepartment: t('grants.unresolvedDepartment'),
    removeGrant: t('actions.removeGrant'),
  };

  const showLoading = isLoading || (isFetching && !runtimeData);
  const errorMessage = error ? getErrorMessage(error) || t('loadError') : null;

  return (
    <div className="space-y-5 p-4">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <h2 className="text-2xl font-bold">{t('title')}</h2>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
            {t('description')}
          </p>
          <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
            {t('policyNote')}
          </p>
        </div>
        <Button
          className="h-9 rounded-md"
          disabled={showLoading || !canManage}
          loading={updateMutation.isPending}
          onClick={() => void handleSave()}
        >
          <Save className="h-4 w-4" />
          {t('actions.save')}
        </Button>
      </div>

      {showLoading ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <Skeleton className="h-28 rounded-lg" />
          <Skeleton className="h-28 rounded-lg" />
          <Skeleton className="h-28 rounded-lg" />
          <Skeleton className="h-28 rounded-lg" />
        </div>
      ) : (
        <div className="space-y-4">
          {errorMessage && (
            <div className="rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {errorMessage}
            </div>
          )}

          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <SurfacePanel
              icon={<Globe2 className="h-5 w-5" />}
              title={t('surfaces.webapp')}
              description={t('surfaces.webappDescription')}
              enabled={webAppEnabled}
              disabled={!canManage}
              source={renderSourceBadge(webAppSurface?.compatibility_source)}
              onChange={setWebAppEnabled}
            />
            <SurfacePanel
              icon={<KeyRound className="h-5 w-5" />}
              title={t('surfaces.api')}
              description={t('surfaces.apiDescription')}
              enabled={apiEnabled}
              disabled={!canManage}
              source={renderSourceBadge(apiSurface?.compatibility_source)}
              onChange={setApiEnabled}
            />
            <SurfacePanel
              icon={<AppWindow className="h-5 w-5" />}
              title={t('surfaces.builtinApp')}
              description={t('surfaces.builtinAppDescription')}
              enabled={builtinEnabled}
              disabled={!canManage}
              source={renderSourceBadge(builtinSurface?.compatibility_source)}
              onChange={setBuiltinEnabled}
            />
            <SurfacePanel
              icon={<Workflow className="h-5 w-5" />}
              title={t('surfaces.internal')}
              description={t('surfaces.internalDescription')}
              enabled={internalSurface?.enabled ?? true}
              disabled
              source={renderSourceBadge(internalSurface?.compatibility_source)}
              status={renderStatusBadge(internalSurface?.enabled ?? true)}
            />
          </div>

          <div
            className={cn(
              'rounded-lg border border-border/80 bg-background p-4 transition-opacity',
              !webAppEnabled && 'opacity-60'
            )}
          >
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <Label className="text-sm font-semibold text-foreground">
                  {t('grants.webappTitle')}
                </Label>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('grants.webappDescription')}
                </p>
              </div>
              {webAppAudienceMode === 'scoped' ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 rounded-md"
                  disabled={!canManage || !webAppEnabled}
                  onClick={addWebAppGrant}
                >
                  <Plus className="h-3.5 w-3.5" />
                  {t('actions.addGrant')}
                </Button>
              ) : null}
            </div>

            <div className="grid gap-2 sm:grid-cols-2">
              <AudienceModeButton
                icon={<Globe2 className="h-4 w-4" />}
                title={t('grants.webappPublic')}
                description={t('grants.webappPublicDescription')}
                selected={webAppAudienceMode === 'public'}
                disabled={!canManage || !webAppEnabled}
                onClick={() => setWebAppAudienceMode('public')}
              />
              <AudienceModeButton
                icon={<Users className="h-4 w-4" />}
                title={t('grants.webappScoped')}
                description={t('grants.webappScopedDescription')}
                selected={webAppAudienceMode === 'scoped'}
                disabled={!canManage || !webAppEnabled}
                onClick={() => setWebAppAudienceMode('scoped')}
              />
            </div>

            {webAppAudienceMode === 'scoped' ? (
              <div className="mt-3 space-y-2">
                {webAppGrants.map((grant, index) => (
                  <RuntimeGrantSubjectRow
                    key={`webapp-${grant.subject_type}-${grant.subject_id}-${index}`}
                    subjectType={grant.subject_type}
                    subjectId={grant.subject_id}
                    disabled={!canManage || !webAppEnabled}
                    canRemove={webAppGrants.length > 1}
                    labels={grantSubjectLabels}
                    onChange={next => updateWebAppGrant(index, next)}
                    onRemove={() => removeWebAppGrant(index)}
                  />
                ))}
              </div>
            ) : null}
          </div>

          <div
            className={cn(
              'rounded-lg border border-border/80 bg-background p-4 transition-opacity',
              !builtinEnabled && 'opacity-60'
            )}
          >
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <Label className="text-sm font-semibold text-foreground">{t('grants.title')}</Label>
                <p className="mt-1 text-xs text-muted-foreground">{t('grants.description')}</p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8 rounded-md"
                disabled={!canManage || !builtinEnabled}
                onClick={addBuiltinGrant}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('actions.addGrant')}
              </Button>
            </div>

            <div className="space-y-2">
              {builtinGrants.map((grant, index) => (
                <RuntimeGrantSubjectRow
                  key={`${grant.subject_type}-${grant.subject_id}-${index}`}
                  subjectType={grant.subject_type}
                  subjectId={grant.subject_id}
                  disabled={!canManage || !builtinEnabled}
                  canRemove={builtinGrants.length > 1}
                  labels={grantSubjectLabels}
                  onChange={next => updateBuiltinGrant(index, next)}
                  onRemove={() => removeBuiltinGrant(index)}
                />
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function SurfacePanel({
  icon,
  title,
  description,
  enabled,
  disabled,
  source,
  status,
  onChange,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  enabled: boolean;
  disabled: boolean;
  source?: ReactNode;
  status?: ReactNode;
  onChange?: (checked: boolean) => void;
}) {
  const t = useT('agents.runtimeAccess');

  return (
    <div className="rounded-lg border border-border/80 bg-background p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-primary/15 bg-primary/10 text-primary">
            {icon}
          </div>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-foreground">{title}</div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
          </div>
        </div>
        {onChange ? (
          <Switch checked={enabled} disabled={disabled} onCheckedChange={onChange} />
        ) : null}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        {status ?? (
          <Badge variant={enabled ? 'success' : 'subtle'}>
            {enabled ? t('status.enabled') : t('status.disabled')}
          </Badge>
        )}
        {source}
      </div>
    </div>
  );
}

function AudienceModeButton({
  icon,
  title,
  description,
  selected,
  disabled,
  onClick,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  selected: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      aria-pressed={selected}
      onClick={onClick}
      className={cn(
        'flex min-h-20 items-start gap-3 rounded-md border p-3 text-left transition-colors',
        selected
          ? 'border-primary bg-primary/5 text-foreground'
          : 'border-border/80 bg-muted/20 text-muted-foreground hover:border-primary/40 hover:bg-primary/5',
        disabled && 'cursor-not-allowed opacity-60 hover:border-border/80 hover:bg-muted/20'
      )}
    >
      <span
        className={cn(
          'mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border',
          selected ? 'border-primary/20 bg-primary/10 text-primary' : 'border-border bg-background'
        )}
      >
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-semibold">{title}</span>
        <span className="mt-1 block text-xs leading-5">{description}</span>
      </span>
    </button>
  );
}
