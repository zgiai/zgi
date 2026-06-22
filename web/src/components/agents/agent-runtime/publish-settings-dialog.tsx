'use client';

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  AlertTriangle,
  Globe2,
  KeyRound,
  LayoutGrid,
  Save,
  ShieldCheck,
  SlidersHorizontal,
  Users,
  Workflow,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  RuntimeAudienceChipList,
  RuntimeAudiencePickerDialog,
  dedupeRuntimeAudienceGrants,
  type RuntimeAudienceGrant,
} from '@/components/runtime-auth/runtime-audience-picker-dialog';
import {
  useAgentRuntimeSurfaces,
  useUpdateAgentRuntimeSurfaces,
} from '@/hooks/agent/use-agent-runtime-surfaces';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  AgentRuntimeGrantSubject,
  AgentRuntimeSurfaceAuthorization,
  UpdateAgentRuntimeSurfaceGrant,
} from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

const EDITABLE_AUDIENCE_SUBJECTS = ['organization', 'department', 'workspace', 'account'] as const;
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
type AudiencePickerTarget = 'webapp' | 'app_center' | null;

interface PublishSettingsState {
  webAppEnabled: boolean;
  webAppAudienceMode: WebAppAudienceMode;
  webAppGrants: RuntimeAudienceGrant[];
  appCenterEnabled: boolean;
  appCenterGrants: RuntimeAudienceGrant[];
  apiEnabled: boolean;
}

interface PublishSettingsDialogProps {
  agentId: string;
  open: boolean;
  canManage?: boolean;
  onOpenChange: (open: boolean) => void;
}

function defaultOrganizationGrant(): RuntimeAudienceGrant {
  return { subject_type: 'organization', subject_id: '' };
}

function isEditableAudienceSubject(
  subject: AgentRuntimeGrantSubject
): subject is EditableAudienceSubject {
  return EDITABLE_AUDIENCE_SUBJECTS.includes(subject as EditableAudienceSubject);
}

function normalizeSubjectId(grant: RuntimeAudienceGrant): string | null {
  return grant.subject_type === 'organization' ? null : grant.subject_id.trim();
}

function findSurface(
  surfaces: AgentRuntimeSurfaceAuthorization[] | undefined,
  name: string
): AgentRuntimeSurfaceAuthorization | null {
  return surfaces?.find(surface => surface.surface === name) ?? null;
}

function editableAudienceGrants(
  surface: AgentRuntimeSurfaceAuthorization | null,
  options: { excludeWorkspaceId?: string | null } = {}
): RuntimeAudienceGrant[] {
  const grants: RuntimeAudienceGrant[] = [];
  const excludedWorkspaceId = options.excludeWorkspaceId?.trim();
  for (const grant of surface?.grants ?? []) {
    if (grant.enabled && isEditableAudienceSubject(grant.subject_type)) {
      if (
        grant.subject_type === 'workspace' &&
        excludedWorkspaceId &&
        grant.subject_id === excludedWorkspaceId
      ) {
        continue;
      }
      grants.push({
        subject_type: grant.subject_type,
        subject_id: grant.subject_type === 'organization' ? '' : (grant.subject_id ?? ''),
      });
    }
  }
  return dedupeRuntimeAudienceGrants(grants);
}

function surfaceHasPublicGrant(surface: AgentRuntimeSurfaceAuthorization | null): boolean {
  return Boolean(surface?.grants.some(grant => grant.enabled && grant.subject_type === 'public'));
}

function surfaceHasEnabledGrant(surface: AgentRuntimeSurfaceAuthorization | null): boolean {
  return Boolean(surface?.grants.some(grant => grant.enabled));
}

function hasOrganizationGrant(grants: RuntimeAudienceGrant[]): boolean {
  return grants.some(grant => grant.subject_type === 'organization');
}

function scopedAudienceGrants(grants: RuntimeAudienceGrant[]): RuntimeAudienceGrant[] {
  return grants.filter(grant => grant.subject_type !== 'organization');
}

function normalizeAudienceSelection(grants: RuntimeAudienceGrant[]): RuntimeAudienceGrant[] {
  const normalized = dedupeRuntimeAudienceGrants(grants);
  if (hasOrganizationGrant(normalized)) {
    return [defaultOrganizationGrant()];
  }
  return scopedAudienceGrants(normalized);
}

function removeAudienceGrant(
  grants: RuntimeAudienceGrant[],
  grantToRemove: RuntimeAudienceGrant
): RuntimeAudienceGrant[] {
  return normalizeAudienceSelection(grants).filter(
    grant =>
      grant.subject_type !== grantToRemove.subject_type ||
      grant.subject_id.trim() !== grantToRemove.subject_id.trim()
  );
}

function serializeAudienceGrants(grants: RuntimeAudienceGrant[]): string[] {
  return normalizeAudienceSelection(grants)
    .map(grant => `${grant.subject_type}:${grant.subject_id.trim()}`)
    .sort();
}

function serializePublishSettingsState(state: PublishSettingsState): string {
  return JSON.stringify({
    webAppEnabled: state.webAppEnabled,
    webAppAudienceMode: state.webAppEnabled ? state.webAppAudienceMode : 'public',
    webAppGrants:
      state.webAppEnabled && state.webAppAudienceMode === 'scoped'
        ? serializeAudienceGrants(state.webAppGrants)
        : [],
    appCenterEnabled: state.appCenterEnabled,
    appCenterGrants: state.appCenterEnabled ? serializeAudienceGrants(state.appCenterGrants) : [],
    apiEnabled: state.apiEnabled,
  });
}

export function PublishSettingsDialog({
  agentId,
  open,
  canManage = true,
  onOpenChange,
}: PublishSettingsDialogProps) {
  const t = useT('agents.runtimeAccess');
  const [webAppEnabled, setWebAppEnabled] = useState(false);
  const [webAppAudienceMode, setWebAppAudienceMode] = useState<WebAppAudienceMode>('public');
  const [webAppGrants, setWebAppGrants] = useState<RuntimeAudienceGrant[]>([]);
  const [appCenterEnabled, setAppCenterEnabled] = useState(false);
  const [appCenterGrants, setAppCenterGrants] = useState<RuntimeAudienceGrant[]>([]);
  const [apiEnabled, setApiEnabled] = useState(false);
  const [pickerTarget, setPickerTarget] = useState<AudiencePickerTarget>(null);
  const [wholeOrganizationTarget, setWholeOrganizationTarget] =
    useState<AudiencePickerTarget>(null);
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false);
  const [baselineState, setBaselineState] = useState<PublishSettingsState | null>(null);

  const { data, error, isLoading, isFetching } = useAgentRuntimeSurfaces(open ? agentId : null);
  const { capabilities } = useAccountCapabilities();
  const updateMutation = useUpdateAgentRuntimeSurfaces();
  const runtimeData = data?.data;
  const owningWorkspaceId = runtimeData?.workspace_id ?? null;

  const surfaces = runtimeData?.surfaces;
  const webAppSurface = useMemo(() => findSurface(surfaces, 'webapp'), [surfaces]);
  const appCenterSurface = useMemo(() => findSurface(surfaces, 'app_center'), [surfaces]);
  const apiSurface = useMemo(() => findSurface(surfaces, 'api'), [surfaces]);
  const internalSurface = useMemo(() => findSurface(surfaces, 'internal'), [surfaces]);
  const canUseWholeOrganization =
    capabilities?.organization.role === 'owner' || capabilities?.organization.role === 'admin';

  useEffect(() => {
    if (!open || !runtimeData) {
      return;
    }

    const nextWebAppEnabled = webAppSurface?.enabled ?? false;
    const nextApiEnabled = apiSurface?.enabled ?? false;
    const nextWebAppAudienceMode =
      surfaceHasPublicGrant(webAppSurface) || !surfaceHasEnabledGrant(webAppSurface)
        ? 'public'
        : 'scoped';
    const nextWebAppGrants = editableAudienceGrants(webAppSurface, {
      excludeWorkspaceId: owningWorkspaceId,
    });
    const nextAppCenterEnabled = appCenterSurface?.enabled ?? false;
    const nextAppCenterGrants = editableAudienceGrants(appCenterSurface, {
      excludeWorkspaceId: owningWorkspaceId,
    });
    const nextState: PublishSettingsState = {
      webAppEnabled: nextWebAppEnabled,
      appCenterEnabled: nextAppCenterEnabled,
      appCenterGrants: normalizeAudienceSelection(nextAppCenterGrants),
      apiEnabled: nextApiEnabled,
      webAppAudienceMode: nextWebAppAudienceMode,
      webAppGrants: normalizeAudienceSelection(nextWebAppGrants),
    };

    setWebAppEnabled(nextState.webAppEnabled);
    setAppCenterEnabled(nextState.appCenterEnabled);
    setAppCenterGrants(nextState.appCenterGrants);
    setApiEnabled(nextState.apiEnabled);
    setWebAppAudienceMode(nextState.webAppAudienceMode);
    setWebAppGrants(nextState.webAppGrants);
    setBaselineState(nextState);
  }, [apiSurface, appCenterSurface, open, owningWorkspaceId, runtimeData, webAppSurface]);

  const currentState = useMemo<PublishSettingsState>(
    () => ({
      webAppEnabled,
      webAppAudienceMode,
      webAppGrants,
      appCenterEnabled,
      appCenterGrants,
      apiEnabled,
    }),
    [apiEnabled, appCenterEnabled, appCenterGrants, webAppAudienceMode, webAppEnabled, webAppGrants]
  );

  const hasUnsavedChanges = useMemo(() => {
    if (!open || !baselineState) {
      return false;
    }
    return (
      serializePublishSettingsState(currentState) !== serializePublishSettingsState(baselineState)
    );
  }, [baselineState, currentState, open]);

  const buildEditableAudienceGrants = (
    grants: RuntimeAudienceGrant[],
    grantRequiredMessage: string,
    options: { defaultWorkspaceId?: string | null } = {}
  ): UpdateAgentRuntimeSurfaceGrant[] | null => {
    const normalized = normalizeAudienceSelection(grants);
    if (
      normalized.some(grant => grant.subject_type !== 'organization' && !grant.subject_id.trim())
    ) {
      toast.error(t('validation.subjectIdRequired'));
      return null;
    }

    const defaultWorkspaceId = options.defaultWorkspaceId?.trim();
    const normalizedWithDefault =
      defaultWorkspaceId && !hasOrganizationGrant(normalized)
        ? dedupeRuntimeAudienceGrants([
            ...normalized,
            { subject_type: 'workspace', subject_id: defaultWorkspaceId },
          ])
        : normalized;

    if (normalizedWithDefault.length === 0) {
      toast.error(grantRequiredMessage);
      return null;
    }

    return normalizedWithDefault.map(grant => ({
      subject_type: grant.subject_type,
      subject_id: normalizeSubjectId(grant),
      enabled: true,
    }));
  };

  const buildWebAppGrants = (): UpdateAgentRuntimeSurfaceGrant[] | null => {
    if (!webAppEnabled) {
      return [];
    }
    if (webAppAudienceMode === 'public') {
      return [{ subject_type: 'public', enabled: true }];
    }
    return buildEditableAudienceGrants(webAppGrants, t('validation.webappGrantRequired'), {
      defaultWorkspaceId: owningWorkspaceId,
    });
  };

  const buildAppCenterGrants = (): UpdateAgentRuntimeSurfaceGrant[] | null => {
    if (!appCenterEnabled) {
      return [];
    }
    return buildEditableAudienceGrants(
      appCenterGrants,
      t('validation.appCenterGrantRequired'),
      {
        defaultWorkspaceId: owningWorkspaceId,
      }
    );
  };

  const closeDialog = () => {
    setPickerTarget(null);
    setWholeOrganizationTarget(null);
    setCloseConfirmOpen(false);
    onOpenChange(false);
  };

  const handleSave = async (): Promise<boolean> => {
    if (!canManage) {
      toast.error(t('validation.manageRequired'));
      return false;
    }
    const webApp = buildWebAppGrants();
    if (webApp === null) {
      return false;
    }
    const appCenter = buildAppCenterGrants();
    if (appCenter === null) {
      return false;
    }

    try {
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
              surface: 'app_center',
              enabled: appCenterEnabled,
              grants: appCenter,
            },
            {
              surface: 'api',
              enabled: apiEnabled,
              grants: [{ subject_type: 'public', enabled: apiEnabled }],
            },
            {
              surface: 'internal',
              enabled: true,
              grants: [{ subject_type: 'internal', enabled: true }],
            },
          ],
        },
      });
      setBaselineState(currentState);
      closeDialog();
      return true;
    } catch {
      // Toasts are handled by the mutation hook.
      return false;
    }
  };

  const requestClose = () => {
    if (updateMutation.isPending) {
      return;
    }
    if (hasUnsavedChanges) {
      setCloseConfirmOpen(true);
      return;
    }
    closeDialog();
  };

  const discardAndClose = () => {
    closeDialog();
  };

  const applyWholeOrganization = (target: AudiencePickerTarget) => {
    if (target === 'webapp') {
      setWebAppAudienceMode('scoped');
      setWebAppGrants([defaultOrganizationGrant()]);
    } else if (target === 'app_center') {
      setAppCenterGrants([defaultOrganizationGrant()]);
    }
    setWholeOrganizationTarget(null);
  };

  const requestWholeOrganization = (target: AudiencePickerTarget) => {
    const grants = target === 'webapp' ? webAppGrants : appCenterGrants;
    if (!canUseWholeOrganization || !target || hasOrganizationGrant(grants)) {
      return;
    }
    setWholeOrganizationTarget(target);
  };

  const renderSourceBadge = (source?: string) =>
    source ? (
      <Badge variant="outline">{t(SOURCE_LABEL_KEYS[source] ?? 'sources.grant')}</Badge>
    ) : null;

  const showLoading = isLoading || (isFetching && !runtimeData);
  const errorMessage = error ? getErrorMessage(error) || t('loadError') : null;
  const pickerValue =
    pickerTarget === 'webapp'
      ? scopedAudienceGrants(webAppGrants)
      : pickerTarget === 'app_center'
        ? scopedAudienceGrants(appCenterGrants)
        : [];
  const pickerTitle =
    pickerTarget === 'app_center' ? t('picker.appCenterDialogTitle') : t('picker.webappDialogTitle');

  const handlePickerConfirm = (next: RuntimeAudienceGrant[]) => {
    const normalized = normalizeAudienceSelection(next);
    if (pickerTarget === 'webapp') {
      setWebAppAudienceMode('scoped');
      setWebAppGrants(normalized);
    } else if (pickerTarget === 'app_center') {
      setAppCenterGrants(normalized);
    }
  };

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={nextOpen => {
          if (nextOpen) {
            onOpenChange(true);
            return;
          }
          requestClose();
        }}
      >
        <DialogContent size="xl" className="p-0 text-left">
          <DialogHeader>
            <DialogTitle>{t('dialogTitle')}</DialogTitle>
            <DialogDescription>{t('dialogDescription')}</DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-4">
            {errorMessage ? (
              <div className="rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                {errorMessage}
              </div>
            ) : null}

            {showLoading ? (
              <div className="space-y-3">
                <Skeleton className="h-24 rounded-md" />
                <Skeleton className="h-28 rounded-md" />
                <Skeleton className="h-28 rounded-md" />
                <Skeleton className="h-20 rounded-md" />
              </div>
            ) : (
              <div className="space-y-3">
                <SurfaceSettingsRow
                  icon={<Globe2 className="h-5 w-5" />}
                  title={t('surfaces.webapp')}
                  description={t('surfaces.webappDescription')}
                  enabled={webAppEnabled}
                  disabled={!canManage}
                  source={renderSourceBadge(webAppSurface?.compatibility_source)}
                  onChange={setWebAppEnabled}
                >
                  {webAppEnabled ? (
                    <div className="space-y-3">
                      <div className="grid gap-2 sm:grid-cols-2">
                        <AudienceModeButton
                          icon={<Globe2 className="h-4 w-4" />}
                          title={t('grants.webappPublic')}
                          description={t('grants.webappPublicDescription')}
                          selected={webAppAudienceMode === 'public'}
                          disabled={!canManage}
                          onClick={() => setWebAppAudienceMode('public')}
                        />
                        <AudienceModeButton
                          icon={<Users className="h-4 w-4" />}
                          title={t('grants.webappScoped')}
                          description={t('grants.webappScopedDescription')}
                          selected={webAppAudienceMode === 'scoped'}
                          disabled={!canManage}
                          onClick={() => setWebAppAudienceMode('scoped')}
                        />
                      </div>
                      {webAppAudienceMode === 'scoped' ? (
                        <AudienceSummaryPanel
                          title={t('grants.webappTitle')}
                          description={t('grants.webappDescription')}
                          grants={webAppGrants}
                          disabled={!canManage}
                          wholeOrganizationDisabled={!canUseWholeOrganization}
                          emptyText={t('picker.emptySelected')}
                          onUseWholeOrganization={() => requestWholeOrganization('webapp')}
                          onEdit={() => setPickerTarget('webapp')}
                          onRemove={grant =>
                            setWebAppGrants(current => removeAudienceGrant(current, grant))
                          }
                        />
                      ) : null}
                    </div>
                  ) : null}
                </SurfaceSettingsRow>

                <SurfaceSettingsRow
                  icon={<LayoutGrid className="h-5 w-5" />}
                  title={t('surfaces.appCenter')}
                  description={t('surfaces.appCenterDescription')}
                  enabled={appCenterEnabled}
                  disabled={!canManage}
                  source={renderSourceBadge(appCenterSurface?.compatibility_source)}
                  onChange={setAppCenterEnabled}
                >
                  {appCenterEnabled ? (
                    <AudienceSummaryPanel
                      title={t('grants.appCenterTitle')}
                      description={t('grants.appCenterDescription')}
                      grants={appCenterGrants}
                      disabled={!canManage}
                      wholeOrganizationDisabled={!canUseWholeOrganization}
                      emptyText={t('picker.emptySelected')}
                      onUseWholeOrganization={() => requestWholeOrganization('app_center')}
                      onEdit={() => setPickerTarget('app_center')}
                      onRemove={grant =>
                        setAppCenterGrants(current => removeAudienceGrant(current, grant))
                      }
                    />
                  ) : null}
                </SurfaceSettingsRow>

                <SurfaceSettingsRow
                  icon={<KeyRound className="h-5 w-5" />}
                  title={t('surfaces.api')}
                  description={t('surfaces.apiDescription')}
                  enabled={apiEnabled}
                  disabled={!canManage}
                  source={renderSourceBadge(apiSurface?.compatibility_source)}
                  onChange={setApiEnabled}
                />

                <SurfaceSettingsRow
                  icon={<Workflow className="h-5 w-5" />}
                  title={t('surfaces.internal')}
                  description={t('surfaces.internalDescription')}
                  enabled={internalSurface?.enabled ?? true}
                  disabled
                  source={renderSourceBadge(internalSurface?.compatibility_source)}
                  status={
                    <Badge variant="success">
                      <ShieldCheck className="h-3 w-3" />
                      {t('status.enabled')}
                    </Badge>
                  }
                />
              </div>
            )}
          </DialogBody>

          <DialogFooter>
            <Button variant="ghost" onClick={requestClose}>
              {t('actions.cancel')}
            </Button>
            <Button
              loading={updateMutation.isPending}
              disabled={showLoading || !canManage}
              onClick={() => void handleSave()}
            >
              <Save className="h-4 w-4" />
              {t('actions.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {pickerTarget ? (
        <RuntimeAudiencePickerDialog
          open
          title={pickerTitle}
          description={t('picker.rangeDialogDescription')}
          value={pickerValue}
          disabled={!canManage}
          excludeWorkspaceId={owningWorkspaceId}
          onOpenChange={nextOpen => {
            if (!nextOpen) {
              setPickerTarget(null);
            }
          }}
          onConfirm={handlePickerConfirm}
        />
      ) : null}

      <Dialog
        open={Boolean(wholeOrganizationTarget)}
        onOpenChange={nextOpen => {
          if (!nextOpen) {
            setWholeOrganizationTarget(null);
          }
        }}
      >
        <DialogContent size="sm" className="p-0">
          <DialogHeader>
            <DialogTitle>{t('wholeOrganizationConfirm.title')}</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <div className="flex items-start gap-2 rounded-md border border-amber-300/60 bg-amber-50 px-3 py-2 text-sm leading-5 text-amber-800 dark:border-amber-900/70 dark:bg-amber-950/20 dark:text-amber-300">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{t('wholeOrganizationConfirm.description')}</span>
            </div>
          </DialogBody>
          <DialogFooter className="border-t bg-muted/40">
            <Button type="button" variant="ghost" onClick={() => setWholeOrganizationTarget(null)}>
              {t('wholeOrganizationConfirm.cancel')}
            </Button>
            <Button
              type="button"
              onClick={() => applyWholeOrganization(wholeOrganizationTarget)}
              disabled={!wholeOrganizationTarget}
            >
              {t('wholeOrganizationConfirm.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={closeConfirmOpen} onOpenChange={setCloseConfirmOpen}>
        <DialogContent size="sm" className="p-0">
          <DialogHeader>
            <DialogTitle>{t('closeGuard.title')}</DialogTitle>
            <DialogDescription>{t('closeGuard.description')}</DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col gap-2 border-t bg-muted/40 sm:flex-row sm:justify-end">
            <Button variant="outline" onClick={discardAndClose} disabled={updateMutation.isPending}>
              {t('closeGuard.discard')}
            </Button>
            <Button
              variant="ghost"
              onClick={() => setCloseConfirmOpen(false)}
              disabled={updateMutation.isPending}
            >
              {t('closeGuard.cancel')}
            </Button>
            <Button onClick={() => void handleSave()} disabled={updateMutation.isPending}>
              {updateMutation.isPending ? t('actions.saving') : t('closeGuard.saveAndClose')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function SurfaceSettingsRow({
  icon,
  title,
  description,
  enabled,
  disabled,
  source,
  status,
  children,
  onChange,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  enabled: boolean;
  disabled: boolean;
  source?: ReactNode;
  status?: ReactNode;
  children?: ReactNode;
  onChange?: (checked: boolean) => void;
}) {
  const t = useT('agents.runtimeAccess');

  return (
    <section
      className={cn(
        'rounded-md border border-border/80 bg-background p-4',
        !enabled && 'opacity-75'
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-primary/15 bg-primary/10 text-primary">
            {icon}
          </div>
          <div className="min-w-0">
            <div className="text-sm font-semibold text-foreground">{title}</div>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
              {status ?? (
                <Badge variant={enabled ? 'success' : 'subtle'}>
                  {enabled ? t('status.enabled') : t('status.disabled')}
                </Badge>
              )}
              {source}
            </div>
          </div>
        </div>
        {onChange ? (
          <Switch checked={enabled} disabled={disabled} onCheckedChange={onChange} />
        ) : null}
      </div>
      {children ? <div className="mt-4 border-t border-border/70 pt-3">{children}</div> : null}
    </section>
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

function AudienceSummaryPanel({
  title,
  description,
  grants,
  disabled,
  wholeOrganizationDisabled,
  emptyText,
  onUseWholeOrganization,
  onEdit,
  onRemove,
}: {
  title: string;
  description: string;
  grants: RuntimeAudienceGrant[];
  disabled: boolean;
  wholeOrganizationDisabled: boolean;
  emptyText: string;
  onUseWholeOrganization: () => void;
  onEdit: () => void;
  onRemove: (grant: RuntimeAudienceGrant) => void;
}) {
  const t = useT('agents.runtimeAccess');
  const selectedWholeOrganization = hasOrganizationGrant(grants);
  const selectedScopedGrants = scopedAudienceGrants(grants);

  return (
    <div className="rounded-md border border-border/70 bg-muted/20 p-3">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="text-sm font-semibold text-foreground">{title}</div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Button
            type="button"
            variant={selectedWholeOrganization ? 'default' : 'outline'}
            size="sm"
            className="h-8 rounded-md"
            disabled={disabled || wholeOrganizationDisabled}
            title={wholeOrganizationDisabled ? t('validation.wholeOrganizationRequiresAdmin') : undefined}
            onClick={onUseWholeOrganization}
          >
            <Users className="h-3.5 w-3.5" />
            {t('actions.useWholeOrganization')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8 rounded-md"
            disabled={disabled}
            onClick={onEdit}
          >
            <SlidersHorizontal className="h-3.5 w-3.5" />
            {t('actions.editRange')}
          </Button>
        </div>
      </div>
      {selectedWholeOrganization ? (
        <div className="mt-3 flex items-start gap-3 rounded-md border border-primary/20 bg-primary/5 p-3 text-primary">
          <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-primary/20 bg-background">
            <Users className="h-4 w-4" />
          </span>
          <span className="min-w-0">
            <span className="block text-sm font-semibold text-foreground">
              {t('grants.wholeOrganizationSelectedTitle')}
            </span>
            <span className="mt-1 block text-xs leading-5 text-muted-foreground">
              {t('grants.wholeOrganizationSelectedDescription')}
            </span>
          </span>
        </div>
      ) : (
        <RuntimeAudienceChipList
          value={selectedScopedGrants}
          disabled={disabled}
          emptyText={emptyText}
          className="mt-3"
          onRemove={onRemove}
        />
      )}
    </div>
  );
}
