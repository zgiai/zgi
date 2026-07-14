'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { AlertCircle, ArrowRight, Building2, CheckCircle2, Info, Loader2 } from 'lucide-react';
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
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useT } from '@/i18n';
import { useWorkspaceAssetMove } from '@/hooks/organization/use-workspace-asset-move';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type {
  WorkspaceAssetMovePreviewItem,
  WorkspaceAssetMovePreviewResponse,
  WorkspaceAssetMoveType,
} from '@/services/types/organization';
import { getErrorMessage } from '@/utils/error-notifications';

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
  const [targetWorkspace, setTargetWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const [preview, setPreview] = useState<WorkspaceAssetMovePreviewResponse | null>(null);
  const [isPreviewing, setIsPreviewing] = useState(false);
  const previewRequestIdRef = useRef(0);
  const openRef = useRef(open);
  openRef.current = open;
  const { previewMutation, moveMutation } = useWorkspaceAssetMove();
  const excludedWorkspaceIds = useMemo(
    () => (currentWorkspace?.id ? [currentWorkspace.id] : []),
    [currentWorkspace?.id]
  );

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
    previewMutation.reset();
    moveMutation.reset();
    // Reset only on close; mutation objects change as their internal state changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(
    () => () => {
      openRef.current = false;
      previewRequestIdRef.current += 1;
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
  const canSubmit = Boolean(request) && Boolean(preview) && preview?.movable && !isPreviewing;

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      openRef.current = false;
      previewRequestIdRef.current += 1;
    }
    onOpenChange(nextOpen);
  };

  const handleSubmit = async () => {
    if (!request || !canSubmit) return;
    await moveMutation.mutateAsync(request);
    onMoved?.();
    handleOpenChange(false);
  };

  const handleTargetWorkspaceChange = (workspace: WorkspaceSelectorValue) => {
    const requestId = previewRequestIdRef.current + 1;
    previewRequestIdRef.current = requestId;
    setTargetWorkspace(workspace);
    setPreview(null);
    setIsPreviewing(true);

    void previewMutation
      .mutateAsync({
        target_workspace_id: workspace.id,
        items: [{ type: assetType, id: assetId }],
      })
      .then(result => {
        if (openRef.current && previewRequestIdRef.current === requestId) {
          setPreview(result);
        }
      })
      .catch(error => {
        if (openRef.current && previewRequestIdRef.current === requestId) {
          toast.error(getErrorMessage(error) || t('assetMove.previewFailed'));
        }
      })
      .finally(() => {
        if (openRef.current && previewRequestIdRef.current === requestId) {
          setIsPreviewing(false);
        }
      });
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent size="md">
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

            <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] sm:items-end">
              <div className="space-y-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {t('assetMove.currentWorkspace')}
                </span>
                <div className="flex h-10 items-center gap-2 rounded-md border bg-background px-3 text-sm">
                  <Building2 className="h-4 w-4 shrink-0 text-muted-foreground" />
                  <span className="truncate font-medium" title={sourceWorkspaceName}>
                    {sourceWorkspaceName}
                  </span>
                </div>
              </div>

              <ArrowRight className="hidden h-4 w-4 text-muted-foreground sm:mb-3 sm:block" />

              <div className="space-y-2">
                <label className="text-xs font-medium text-muted-foreground">
                  {t('assetMove.targetWorkspace')}
                </label>
                <WorkspaceSelector
                  value={targetWorkspace}
                  onChange={handleTargetWorkspaceChange}
                  placeholder={t('assetMove.targetWorkspacePlaceholder')}
                  excludedWorkspaceIds={excludedWorkspaceIds}
                />
              </div>
            </div>
          </div>

          {isPreviewing && (
            <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t('assetMove.previewing')}
            </div>
          )}

          {!isPreviewing && preview?.movable && blockers.length === 0 && warnings.length === 0 && (
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
          <Button onClick={handleSubmit} disabled={!canSubmit} loading={moveMutation.isPending}>
            {t('assetMove.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
