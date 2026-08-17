'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  BriefcaseBusiness,
  Building2,
  LockKeyhole,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  UserRound,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import {
  integrationConnectionGrantItems,
  useCreateIntegrationConnectionGrant,
  useDeleteIntegrationConnectionGrant,
  useIntegrationConnectionGrants,
  useUpdateIntegrationConnectionGrant,
} from '@/hooks';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useManagedWorkspaces } from '@/hooks/workspace/use-managed-workspaces';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  IntegrationActionDefinition,
  IntegrationConnectionGrant,
  SaveIntegrationConnectionGrantRequest,
} from '@/services/types/integration';
import {
  deriveConnectionGrantAccessMode,
  isConnectionGrantFormValid,
  type GrantPrincipalSelection,
} from './connection-grant-form';
import { GrantPrincipalPicker } from './grant-principal-picker';
import { safeIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';

interface ConnectionGrantsPanelProps {
  connectionId: string;
  actions: IntegrationActionDefinition[];
  enabled?: boolean;
}

const ALL_ACTIONS = '*';

function isNormalWorkspace(workspace: object): boolean {
  return 'status' in workspace && workspace.status === 'normal';
}

function isGrantEditable(grant: IntegrationConnectionGrant): boolean {
  // Missing capability metadata (for example during a rolling deployment
  // against an older API) must fail closed instead of exposing a broken editor.
  return grant.editable === true && !grant.has_resource_constraints;
}

export function IntegrationConnectionGrantsPanel({
  connectionId,
  actions,
  enabled = true,
}: ConnectionGrantsPanelProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const { currentOrganization } = useOrganizations();
  const grantsQuery = useIntegrationConnectionGrants(connectionId, enabled);
  const createMutation = useCreateIntegrationConnectionGrant(connectionId);
  const updateMutation = useUpdateIntegrationConnectionGrant(connectionId);
  const deleteMutation = useDeleteIntegrationConnectionGrant(connectionId);
  const [editingGrant, setEditingGrant] = useState<IntegrationConnectionGrant | null>(null);
  const [editorOpen, setEditorOpen] = useState(false);
  const [deleteGrant, setDeleteGrant] = useState<IntegrationConnectionGrant | null>(null);
  const grants = integrationConnectionGrantItems(grantsQuery.data?.data);

  const workspacesQuery = useManagedWorkspaces(
    currentOrganization?.id ?? '',
    editorOpen && Boolean(currentOrganization?.id)
  );
  const eligibleWorkspaces = useMemo(
    () => (workspacesQuery.data ?? []).filter(isNormalWorkspace),
    [workspacesQuery.data]
  );
  const workspaceNames = useMemo(
    () => new Map(eligibleWorkspaces.map(workspace => [workspace.id, workspace.name])),
    [eligibleWorkspaces]
  );
  const actionNames = new Map(actions.map(action => [action.id, metadata.actionName(action)]));

  const principalLabel = (grant: IntegrationConnectionGrant) => {
    if (grant.principal_type === 'organization') {
      return safeIntegrationDisplayText(
        currentOrganization?.name,
        t('grants.organizationPrincipal')
      );
    }
    if (grant.principal_display_name) {
      return safeIntegrationDisplayText(
        grant.principal_display_name,
        t(`grants.principalState.missing.${grant.principal_type}`)
      );
    }
    if (grant.principal_type === 'workspace') {
      const workspaceName = workspaceNames.get(grant.principal_id ?? '');
      if (workspaceName) {
        return safeIntegrationDisplayText(
          workspaceName,
          t('grants.principalState.missing.workspace')
        );
      }
    }
    return t(`grants.principalState.missing.${grant.principal_type}`);
  };

  const openCreate = () => {
    setEditingGrant(null);
    setEditorOpen(true);
  };
  const openEdit = (grant: IntegrationConnectionGrant) => {
    if (!isGrantEditable(grant)) return;
    setEditingGrant(grant);
    setEditorOpen(true);
  };

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">{t('grants.title')}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t('grants.description')}</p>
        </div>
        <Button size="sm" variant="outline" onClick={openCreate}>
          <Plus className="size-4" />
          {t('grants.add')}
        </Button>
      </div>

      <div className="rounded-lg border">
        {grantsQuery.isLoading ? (
          <div className="space-y-2 p-3">
            <Skeleton className="h-16 rounded-md" />
            <Skeleton className="h-16 rounded-md" />
          </div>
        ) : grantsQuery.isError ? (
          <div className="p-5 text-center">
            <p className="text-sm text-destructive">{t('grants.loadFailed')}</p>
            <Button
              className="mt-3"
              size="sm"
              variant="outline"
              onClick={() => void grantsQuery.refetch()}
            >
              <RefreshCw className="size-4" />
              {t('connections.retry')}
            </Button>
          </div>
        ) : grants.length === 0 ? (
          <div className="p-6 text-center text-muted-foreground">
            <ShieldCheck className="mx-auto size-6" />
            <p className="mt-2 text-sm">{t('grants.empty')}</p>
          </div>
        ) : (
          <div className="divide-y">
            {grants.map(grant => (
              <div key={grant.id} className="flex flex-col gap-3 p-3 sm:flex-row sm:items-start">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="truncate text-sm font-medium">{principalLabel(grant)}</span>
                    <Badge variant="outline">{t(`grants.principal.${grant.principal_type}`)}</Badge>
                    <Badge variant="subtle">{t(`grants.accessMode.${grant.access_mode}`)}</Badge>
                    {grant.principal_state === 'missing' ? (
                      <Badge variant="destructive">{t('grants.principalState.missingBadge')}</Badge>
                    ) : null}
                    {!isGrantEditable(grant) ? (
                      <Badge variant="warning">
                        <LockKeyhole />
                        {t('grants.readOnlyGrant')}
                      </Badge>
                    ) : null}
                  </div>
                  {!isGrantEditable(grant) ? (
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {grant.has_resource_constraints
                        ? t('grants.resourceConstrainedDescription')
                        : t('grants.readOnlyDescription')}
                    </p>
                  ) : null}
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {grant.allowed_action_ids.map(actionId => (
                      <Badge key={actionId} variant="subtle">
                        {actionId === ALL_ACTIONS
                          ? t('grants.allActions')
                          : safeIntegrationDisplayText(
                              actionNames.get(actionId),
                              t('common.unknownAction')
                            )}
                      </Badge>
                    ))}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button
                    isIcon
                    size="sm"
                    variant="ghost"
                    aria-label={t('grants.edit')}
                    title={!isGrantEditable(grant) ? t('grants.editDisabled') : undefined}
                    disabled={!isGrantEditable(grant)}
                    onClick={() => openEdit(grant)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    isIcon
                    size="sm"
                    variant="ghost"
                    aria-label={t('grants.delete')}
                    onClick={() => setDeleteGrant(grant)}
                  >
                    <Trash2 className="size-4 text-destructive" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <ConnectionGrantEditor
        open={editorOpen}
        grant={editingGrant}
        actions={actions}
        organizationName={currentOrganization?.name ?? ''}
        workspaces={eligibleWorkspaces.map(workspace => ({
          id: workspace.id,
          label: safeIntegrationDisplayText(
            workspace.name,
            t('grants.principalPicker.unnamed.workspace')
          ),
        }))}
        workspacesLoading={workspacesQuery.isLoading || workspacesQuery.isFetching}
        isSubmitting={createMutation.isPending || updateMutation.isPending}
        onOpenChange={setEditorOpen}
        onSubmit={async data => {
          if (editingGrant) {
            await updateMutation.mutateAsync({ grantId: editingGrant.id, data });
          } else {
            await createMutation.mutateAsync(data);
          }
          setEditorOpen(false);
          setEditingGrant(null);
        }}
      />

      <ConfirmDialog
        variant="danger"
        open={Boolean(deleteGrant)}
        onOpenChange={open => {
          if (!open) setDeleteGrant(null);
        }}
        title={t('grants.deleteTitle')}
        description={t('grants.deleteDescription')}
        cancelText={t('delete.cancel')}
        confirmText={t('grants.delete')}
        loading={deleteMutation.isPending}
        onConfirm={() => {
          if (!deleteGrant) return;
          deleteMutation.mutate(deleteGrant.id, {
            onSuccess: () => setDeleteGrant(null),
          });
        }}
      />
    </section>
  );
}

interface ConnectionGrantEditorProps {
  open: boolean;
  grant: IntegrationConnectionGrant | null;
  actions: IntegrationActionDefinition[];
  organizationName: string;
  workspaces: Array<{ id: string; label: string }>;
  workspacesLoading: boolean;
  isSubmitting: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: SaveIntegrationConnectionGrantRequest) => Promise<void>;
}

function ConnectionGrantEditor({
  open,
  grant,
  actions,
  organizationName,
  workspaces,
  workspacesLoading,
  isSubmitting,
  onOpenChange,
  onSubmit,
}: ConnectionGrantEditorProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [principalType, setPrincipalType] = useState<GrantPrincipalSelection>('');
  const [principalId, setPrincipalId] = useState('');
  const [principalDisplayName, setPrincipalDisplayName] = useState('');
  const [actionIds, setActionIds] = useState<string[]>([]);
  const [validationError, setValidationError] = useState(false);

  useEffect(() => {
    if (!open) return;
    setPrincipalType(grant?.principal_type ?? '');
    setPrincipalId(grant?.principal_id ?? '');
    setPrincipalDisplayName(grant?.principal_display_name ?? '');
    const savedActionIds = grant?.allowed_action_ids ?? [];
    setActionIds(
      savedActionIds.includes(ALL_ACTIONS) ? actions.map(action => action.id) : savedActionIds
    );
    setValidationError(false);
  }, [actions, grant, open]);

  const accessMode = useMemo(
    () => deriveConnectionGrantAccessMode(actions, actionIds),
    [actionIds, actions]
  );
  const unknownActionIds = useMemo(() => {
    const knownIds = new Set(actions.map(action => action.id));
    return actionIds.filter(actionId => !knownIds.has(actionId));
  }, [actionIds, actions]);

  const setAction = (actionId: string, checked: boolean) => {
    setValidationError(false);
    setActionIds(current => {
      const withoutLegacyWildcard = current.filter(id => id !== ALL_ACTIONS);
      return checked
        ? [...withoutLegacyWildcard.filter(id => id !== actionId), actionId]
        : withoutLegacyWildcard.filter(id => id !== actionId);
    });
  };

  const submit = async () => {
    const normalizedPrincipalId = principalId.trim();
    if (
      unknownActionIds.length > 0 ||
      !isConnectionGrantFormValid(principalType, normalizedPrincipalId, actionIds)
    ) {
      setValidationError(true);
      return;
    }
    try {
      await onSubmit({
        revision: grant?.revision ?? 0,
        principal_type: principalType,
        principal_id: principalType === 'organization' ? null : normalizedPrincipalId,
        access_mode: accessMode,
        allowed_action_ids: actionIds,
      });
    } catch {
      // Mutation hooks surface the API error and keep the editor open.
    }
  };

  const summaryPrincipalName =
    principalType === 'organization'
      ? safeIntegrationDisplayText(organizationName, t('grants.organizationPrincipal'))
      : safeIntegrationDisplayText(
          principalDisplayName,
          principalType
            ? t(`grants.principalPicker.unnamed.${principalType}`)
            : t('grants.summary.pendingPrincipal')
        );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md" className="z-[60] p-0" overlayClassName="z-[60]">
        <DialogHeader>
          <DialogTitle>{t(grant ? 'grants.editTitle' : 'grants.createTitle')}</DialogTitle>
          <DialogDescription>{t('grants.editorDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5 pb-4">
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">{t('grants.scopeLabel')}</legend>
            <div className="grid gap-2">
              {(
                [
                  ['organization', Building2],
                  ['workspace', BriefcaseBusiness],
                  ['account', UserRound],
                ] as const
              ).map(([value, Icon]) => {
                const selected = principalType === value;
                return (
                  <label
                    key={value}
                    className={cn(
                      'flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/40',
                      selected && 'border-primary bg-primary/5'
                    )}
                  >
                    <input
                      type="radio"
                      name="grant-principal-type"
                      value={value}
                      checked={selected}
                      className="peer sr-only"
                      onChange={() => {
                        setPrincipalType(value);
                        setPrincipalId('');
                        setPrincipalDisplayName('');
                        setValidationError(false);
                      }}
                    />
                    <span
                      aria-hidden="true"
                      className={cn(
                        'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border border-muted-foreground/50 peer-focus-visible:ring-2 peer-focus-visible:ring-ring peer-focus-visible:ring-offset-2',
                        selected && 'border-primary'
                      )}
                    >
                      <span
                        className={cn(
                          'size-2 rounded-full bg-primary opacity-0',
                          selected && 'opacity-100'
                        )}
                      />
                    </span>
                    <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                    <span className="min-w-0">
                      <span className="block text-sm font-medium">
                        {t(`grants.scope.${value}.title`)}
                      </span>
                      <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
                        {t(`grants.scope.${value}.description`)}
                      </span>
                    </span>
                  </label>
                );
              })}
            </div>
          </fieldset>

          {principalType === 'workspace' || principalType === 'account' ? (
            <div className="space-y-2">
              <Label>{t(`grants.principalPicker.label.${principalType}`)}</Label>
              <GrantPrincipalPicker
                principalType={principalType}
                value={principalId}
                initialLabel={
                  grant?.principal_type === principalType && grant.principal_id === principalId
                    ? grant.principal_display_name
                    : null
                }
                initialState={
                  grant?.principal_type === principalType && grant.principal_id === principalId
                    ? grant.principal_state
                    : undefined
                }
                workspaces={workspaces}
                workspacesLoading={workspacesLoading}
                onChange={(value, label) => {
                  setPrincipalId(value);
                  setPrincipalDisplayName(label);
                  setValidationError(false);
                }}
              />
            </div>
          ) : null}

          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 p-3">
            <div>
              <p className="text-sm font-medium">{t('grants.permissionLevel')}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t('grants.permissionLevelDescription')}
              </p>
            </div>
            <Badge
              variant={
                actionIds.length === 0 ? 'subtle' : accessMode === 'read' ? 'success' : 'warning'
              }
            >
              {actionIds.length === 0
                ? t('grants.permissionPending')
                : t(`grants.accessMode.${accessMode}`)}
            </Badge>
          </div>

          <div className="space-y-2">
            <Label>{t('grants.actionsLabel')}</Label>
            <div className="max-h-56 space-y-2 overflow-y-auto rounded-lg border p-3">
              {actions.map(action => {
                const actionLabel = metadata.actionName(action);
                return (
                  <div
                    key={action.id}
                    className="flex items-start gap-2 rounded-md p-2 text-sm hover:bg-muted/40"
                  >
                    <Checkbox
                      checked={actionIds.includes(action.id)}
                      aria-label={actionLabel}
                      onCheckedChange={checked => setAction(action.id, checked === true)}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="flex flex-wrap items-center gap-1.5">
                        <span className="font-medium">{actionLabel}</span>
                        <Badge variant="outline">{metadata.effect(action.effect)}</Badge>
                        <Badge variant="subtle">{metadata.risk(action.risk_level)}</Badge>
                      </span>
                      {metadata.actionDescription(action) ? (
                        <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                          {metadata.actionDescription(action)}
                        </span>
                      ) : null}
                    </span>
                  </div>
                );
              })}
              {unknownActionIds.map(actionId => (
                <div
                  key={actionId}
                  className="flex items-start gap-2 rounded-md border border-dashed border-amber-300/70 p-2 text-sm dark:border-amber-900/70"
                >
                  <Checkbox
                    checked
                    aria-label={t('grants.unavailableAction')}
                    onCheckedChange={checked => setAction(actionId, checked === true)}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="font-medium">{t('grants.unavailableAction')}</span>
                  </span>
                </div>
              ))}
            </div>
            {unknownActionIds.length > 0 ? (
              <div
                className="rounded-md border border-amber-300/70 bg-amber-50/60 px-3 py-2 text-sm leading-5 text-amber-700 dark:border-amber-900/70 dark:bg-amber-950/20 dark:text-amber-300"
                role="alert"
              >
                {t('grants.unknownActionsBlocking')}
              </div>
            ) : null}
            <p className="text-xs leading-5 text-muted-foreground">
              {t('grants.explicitActionsNotice')}
            </p>
          </div>

          {validationError ? (
            <p className="text-sm text-destructive" role="alert">
              {t('grants.validationError')}
            </p>
          ) : null}

          <div className="space-y-2" aria-live="polite">
            {principalType && actionIds.length > 0 ? (
              <div className="rounded-lg border border-primary/20 bg-primary/5 p-3 text-sm leading-6">
                {t(`grants.summary.${principalType}`, {
                  principal: summaryPrincipalName || t('grants.summary.pendingPrincipal'),
                  count: actionIds.length,
                  permission: t(`grants.accessMode.${accessMode}`),
                })}
              </div>
            ) : null}
            <p className="text-xs leading-5 text-muted-foreground">{t('grants.unionNotice')}</p>
          </div>
        </DialogBody>
        <DialogFooter className="border-t bg-muted/20">
          <Button variant="ghost" disabled={isSubmitting} onClick={() => onOpenChange(false)}>
            {t('dialog.cancel')}
          </Button>
          <Button
            disabled={isSubmitting || unknownActionIds.length > 0}
            onClick={() => void submit()}
          >
            {t('grants.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
