'use client';

import Link from 'next/link';
import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ArrowDown, CheckCircle2, Info, Loader2, Plus, Users } from 'lucide-react';
import { toast } from 'sonner';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { AgentResourceBoundDialog } from '@/components/common/agent-resource-bound-dialog';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useT } from '@/i18n';
import {
  useWorkspaceAssetMove,
  useWorkspaceAssetMoveEligibleTargets,
} from '@/hooks/organization/use-workspace-asset-move';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useCurrentWorkspace, usePermissions } from '@/store/workspace-store';
import type {
  WorkspaceAssetMovePreviewItem,
  WorkspaceAssetMovePreviewResponse,
  WorkspaceAssetMoveDependencyPreviewResponse,
  WorkspaceAssetMoveType,
} from '@/services/types/organization';
import { getErrorMessage } from '@/utils/error-notifications';
import type { AgentResourceBoundImpact } from '@/services/types/common';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';

interface WorkspaceAssetMoveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  assetType: WorkspaceAssetMoveType;
  assetId: string;
  assetName?: string;
  onMoved?: () => void;
}

export function WorkspaceAssetMoveDialog({
  open,
  onOpenChange,
  assetType,
  assetId,
  assetName,
  onMoved,
}: WorkspaceAssetMoveDialogProps) {
  const t = useT('common');
  const currentWorkspace = useCurrentWorkspace();
  const permissions = usePermissions();
  const { currentOrganization } = useOrganizations();
  const moveItems = useMemo(() => [{ type: assetType, id: assetId }], [assetId, assetType]);
  const eligibleTargetsQuery = useWorkspaceAssetMoveEligibleTargets(moveItems, open);
  const [targetWorkspace, setTargetWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const [preview, setPreview] = useState<WorkspaceAssetMovePreviewResponse | null>(null);
  const [isPreviewing, setIsPreviewing] = useState(false);
  const [preflightReady, setPreflightReady] = useState(false);
  const [dependencyImpact, setDependencyImpact] = useState<
    WorkspaceAssetMoveDependencyPreviewResponse['agent_binding_impact'] | null
  >(null);
  const [preflightBindingOpen, setPreflightBindingOpen] = useState(false);
  const [acknowledgedDependencyAgentIds, setAcknowledgedDependencyAgentIds] = useState<string[]>(
    []
  );
  const previewRequestIdRef = useRef(0);
  const dependencyRequestIdRef = useRef(0);
  const dependencyPreflightKeyRef = useRef('');
  const openRef = useRef(open);
  openRef.current = open;
  const onOpenChangeRef = useRef(onOpenChange);
  onOpenChangeRef.current = onOpenChange;
  const [bindingImpact, setBindingImpact] = useState<AgentResourceBoundImpact | null>(null);
  const [bindingConfirmOpen, setBindingConfirmOpen] = useState(false);
  const { dependencyMutation, previewMutation, moveMutation } = useWorkspaceAssetMove();
  const dependencyMutationRef = useRef(dependencyMutation);
  dependencyMutationRef.current = dependencyMutation;
  const preflightToastId = useMemo(
    () => `workspace-asset-move-preflight:${assetType}:${assetId}`,
    [assetId, assetType]
  );
  const excludedWorkspaceIds = useMemo(
    () => (currentWorkspace?.id ? [currentWorkspace.id] : []),
    [currentWorkspace?.id]
  );
  const eligibleTargets = eligibleTargetsQuery.data ?? [];
  const hasNoTargetWorkspaces =
    !eligibleTargetsQuery.isLoading && !eligibleTargetsQuery.error && eligibleTargets.length === 0;
  const organizationRole =
    permissions.organizationRole ?? currentOrganization?.organization_role ?? null;
  const canCreateWorkspace = organizationRole === 'owner' || organizationRole === 'admin';

  useEffect(() => {
    if (!open) {
      dependencyPreflightKeyRef.current = '';
      dependencyRequestIdRef.current += 1;
      setPreflightReady(false);
      toast.dismiss(preflightToastId);
      return;
    }

    const preflightKey = `${assetType}:${assetId}`;
    if (dependencyPreflightKeyRef.current === preflightKey) return;
    dependencyPreflightKeyRef.current = preflightKey;
    const requestId = dependencyRequestIdRef.current + 1;
    dependencyRequestIdRef.current = requestId;
    setPreflightReady(false);
    toast.loading(t('assetMove.preflightChecking'), { id: preflightToastId });
    void dependencyMutationRef.current
      .mutateAsync({ items: moveItems })
      .then(result => {
        if (!openRef.current || dependencyRequestIdRef.current !== requestId) return;
        const nextImpact = result.agent_binding_impact ?? null;
        setDependencyImpact(nextImpact);
        setAcknowledgedDependencyAgentIds([]);
        setPreflightReady(true);
        setPreflightBindingOpen(Boolean(nextImpact?.agents.length));
        toast.dismiss(preflightToastId);
      })
      .catch(error => {
        if (!openRef.current || dependencyRequestIdRef.current !== requestId) return;
        toast.dismiss(preflightToastId);
        toast.error(getErrorMessage(error) || t('assetMove.dependencyPreflightFailed'));
        onOpenChangeRef.current(false);
      });
  }, [assetId, assetType, moveItems, open, preflightToastId, t]);

  const request = useMemo(() => {
    if (!targetWorkspace?.id) return null;
    return {
      target_workspace_id: targetWorkspace.id,
      items: [{ type: assetType, id: assetId }],
    };
  }, [assetId, assetType, targetWorkspace?.id]);

  useEffect(() => {
    if (open) return;
    previewRequestIdRef.current += 1;
    setTargetWorkspace(undefined);
    setPreview(null);
    setIsPreviewing(false);
    setDependencyImpact(null);
    setPreflightBindingOpen(false);
    setAcknowledgedDependencyAgentIds([]);
    setBindingImpact(null);
    setBindingConfirmOpen(false);
    dependencyMutationRef.current.reset();
    previewMutation.reset();
    moveMutation.reset();
    // Reset only on close; mutation objects change as their internal state changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(
    () => () => {
      openRef.current = false;
      previewRequestIdRef.current += 1;
      dependencyRequestIdRef.current += 1;
    },
    []
  );

  const previewItems: WorkspaceAssetMovePreviewItem[] = preview?.items ?? [];
  const previewItem = previewItems[0];
  const blockers: string[] = previewItems.flatMap(item => item.blockers ?? []);
  const warnings: string[] = previewItems.flatMap(item => item.warnings ?? []);
  const sourceWorkspaceName =
    previewItem?.from_workspace?.name ||
    currentWorkspace?.name ||
    previewItem?.from_workspace?.id ||
    currentWorkspace?.id ||
    t('assetMove.unknownWorkspace');
  const canSubmit = Boolean(request) && preflightReady && !isPreviewing;
  const acknowledgedDependencyAgentIdSet = useMemo(
    () => new Set(acknowledgedDependencyAgentIds),
    [acknowledgedDependencyAgentIds]
  );
  const hasAcknowledgedImpact = (impact: AgentResourceBoundImpact) =>
    impact.agents.length > 0 &&
    impact.agents.every(agent => acknowledgedDependencyAgentIdSet.has(agent.agent_id));

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      openRef.current = false;
      previewRequestIdRef.current += 1;
    }
    onOpenChange(nextOpen);
  };

  const handleSubmit = async (impact?: AgentResourceBoundImpact, validatedPreview = preview) => {
    if (!request || !validatedPreview?.movable) return;
    try {
      await moveMutation.mutateAsync({
        ...request,
        agent_binding_action: impact ? 'unbind' : undefined,
        impact_token: impact?.impact_token,
      });
      setBindingImpact(null);
      onMoved?.();
      handleOpenChange(false);
    } catch (error) {
      const nextImpact = getAgentResourceBoundImpact(error);
      if (nextImpact) {
        setBindingImpact(nextImpact);
        setBindingConfirmOpen(true);
      }
    }
  };

  const handleConfirmMove = async () => {
    if (!request || !canSubmit) return;

    if (preview?.movable && warnings.length > 0) {
      if (bindingImpact) {
        if (hasAcknowledgedImpact(bindingImpact)) {
          await handleSubmit(bindingImpact, preview);
          return;
        }
        setBindingConfirmOpen(true);
        return;
      }
      await handleSubmit(undefined, preview);
      return;
    }

    const requestId = previewRequestIdRef.current + 1;
    previewRequestIdRef.current = requestId;
    setIsPreviewing(true);
    setBindingConfirmOpen(false);

    try {
      const latestPreview = await previewMutation.mutateAsync(request);
      if (!openRef.current || previewRequestIdRef.current !== requestId) return;

      const latestImpact = latestPreview.agent_binding_impact ?? null;
      setPreview(latestPreview);
      setBindingImpact(latestImpact);
      if (!latestPreview.movable) return;

      if (latestPreview.items.some(item => (item.warnings ?? []).length > 0)) return;

      if (latestImpact) {
        if (hasAcknowledgedImpact(latestImpact)) {
          await handleSubmit(latestImpact, latestPreview);
          return;
        }
        setBindingConfirmOpen(true);
        return;
      }
      await handleSubmit(undefined, latestPreview);
    } catch (error) {
      if (openRef.current && previewRequestIdRef.current === requestId) {
        toast.error(getErrorMessage(error) || t('assetMove.previewFailed'));
      }
    } finally {
      if (openRef.current && previewRequestIdRef.current === requestId) {
        setIsPreviewing(false);
      }
    }
  };

  const handleTargetWorkspaceChange = (workspace: WorkspaceSelectorValue) => {
    previewRequestIdRef.current += 1;
    setTargetWorkspace(workspace);
    setPreview(null);
    setBindingImpact(null);
    setBindingConfirmOpen(false);
    setIsPreviewing(false);
  };

  return (
    <>
      <Dialog
        open={open && preflightReady && !preflightBindingOpen && !bindingConfirmOpen}
        onOpenChange={handleOpenChange}
      >
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('assetMove.title')}</DialogTitle>
            <DialogDescription>
              {assetName
                ? t('assetMove.descriptionWithName', { name: assetName })
                : t('assetMove.description')}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="space-y-3 rounded-lg border bg-muted/20 p-4">
              <div>
                <h3 className="text-sm font-medium text-foreground">
                  {t('assetMove.locationTitle')}
                </h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('assetMove.locationDescription')}
                </p>
              </div>

              <div className="space-y-3">
                <div className="space-y-2">
                  <span className="text-xs font-medium text-muted-foreground">
                    {t('assetMove.currentWorkspace')}
                  </span>
                  <div className="flex min-h-10 items-center gap-2 px-1 text-sm">
                    <Users className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium" title={sourceWorkspaceName}>
                      {sourceWorkspaceName}
                    </span>
                  </div>
                </div>

                <div className="flex justify-center" aria-hidden="true">
                  <div className="flex size-7 items-center justify-center rounded-full border bg-background text-muted-foreground">
                    <ArrowDown className="size-4" />
                  </div>
                </div>

                <div className="space-y-2">
                  <label className="text-xs font-medium text-muted-foreground">
                    {t('assetMove.targetWorkspace')}
                  </label>
                  <WorkspaceSelector
                    value={targetWorkspace}
                    onChange={handleTargetWorkspaceChange}
                    placeholder={t('assetMove.targetWorkspacePlaceholder')}
                    excludedWorkspaceIds={excludedWorkspaceIds}
                    workspaceOptions={eligibleTargets}
                    workspaceOptionsLoading={eligibleTargetsQuery.isFetching}
                    disabled={hasNoTargetWorkspaces || Boolean(eligibleTargetsQuery.error)}
                  />
                </div>
              </div>

              {eligibleTargetsQuery.error && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>{t('assetMove.targetLoadFailedTitle')}</AlertTitle>
                  <AlertDescription>
                    {getErrorMessage(eligibleTargetsQuery.error) ||
                      t('assetMove.targetLoadFailedDescription')}
                  </AlertDescription>
                </Alert>
              )}

              {hasNoTargetWorkspaces && (
                <div className="rounded-md border border-dashed bg-background px-4 py-3">
                  <div className="text-sm font-medium text-foreground">
                    {t('assetMove.noTargetWorkspaceTitle')}
                  </div>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {canCreateWorkspace
                      ? t('assetMove.noTargetWorkspaceAdminDescription')
                      : t('assetMove.noTargetWorkspaceMemberDescription')}
                  </p>
                  {canCreateWorkspace && (
                    <Button asChild type="button" variant="outline" size="sm" className="mt-3">
                      <Link href="/dashboard/organization/workspaces?createWorkspace=1">
                        <Plus className="size-4" />
                        {t('assetMove.createWorkspace')}
                      </Link>
                    </Button>
                  )}
                </div>
              )}
            </div>

            {isPreviewing && (
              <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('assetMove.previewing')}
              </div>
            )}

            {!isPreviewing &&
              !bindingImpact &&
              preview?.movable &&
              blockers.length === 0 &&
              warnings.length === 0 && (
                <Alert className="border-emerald-300/70 bg-emerald-50/70 text-emerald-950 dark:bg-emerald-950/20 dark:text-emerald-100">
                  <CheckCircle2 className="h-4 w-4" />
                  <AlertTitle>{t('assetMove.readyTitle')}</AlertTitle>
                  <AlertDescription>{t('assetMove.readyDescription')}</AlertDescription>
                </Alert>
              )}

            {!isPreviewing && blockers.length > 0 && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>{t('assetMove.blockersTitle')}</AlertTitle>
                <AlertDescription>
                  <ul className="list-disc space-y-1 pl-4">
                    {blockers.map((blocker, index) => (
                      <li key={`${blocker}-${index}`}>{blocker}</li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}

            {!isPreviewing && warnings.length > 0 && blockers.length === 0 && (
              <Alert className="border-amber-300/70 bg-amber-50/70 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">
                <Info className="h-4 w-4" />
                <AlertTitle>{t('assetMove.warningsTitle')}</AlertTitle>
                <AlertDescription>
                  <ul className="list-disc space-y-1 pl-4">
                    {warnings.map((warning, index) => (
                      <li key={`${warning}-${index}`}>{warning}</li>
                    ))}
                  </ul>
                </AlertDescription>
              </Alert>
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleOpenChange(false)}>
              {t('cancel')}
            </Button>
            <Button
              onClick={() => void handleConfirmMove()}
              disabled={!canSubmit}
              loading={isPreviewing || moveMutation.isPending}
            >
              {t('assetMove.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AgentResourceBoundDialog
        open={open && preflightBindingOpen && Boolean(dependencyImpact?.agents.length)}
        agents={dependencyImpact?.agents}
        actionLabel={t('assetMove.continueToTargetSelection')}
        actionVariant="default"
        warningTitle={t('assetMove.bindingPreflightWarningTitle')}
        warningDescription={t('assetMove.bindingPreflightWarningDescription')}
        onOpenChange={nextOpen => {
          if (nextOpen) {
            setPreflightBindingOpen(true);
            return;
          }
          handleOpenChange(false);
        }}
        onConfirm={() => {
          setAcknowledgedDependencyAgentIds(
            (dependencyImpact?.agents ?? []).map(agent => agent.agent_id)
          );
          setPreflightBindingOpen(false);
        }}
      />
      <AgentResourceBoundDialog
        open={bindingConfirmOpen && Boolean(bindingImpact)}
        impact={bindingImpact}
        loading={moveMutation.isPending}
        actionLabel={t('assetMove.unbindAndMove')}
        onOpenChange={setBindingConfirmOpen}
        onConfirm={() => {
          if (bindingImpact) void handleSubmit(bindingImpact);
        }}
      />
    </>
  );
}
