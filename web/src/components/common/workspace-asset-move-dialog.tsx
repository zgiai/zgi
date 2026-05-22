'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, ArrowRight, Info, Loader2 } from 'lucide-react';
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
import type {
  WorkspaceAssetMovePreviewItem,
  WorkspaceAssetMoveType,
} from '@/services/types/organization';

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
  const [targetWorkspace, setTargetWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const { previewMutation, moveMutation } = useWorkspaceAssetMove();

  const request = useMemo(() => {
    if (!targetWorkspace?.id) return null;
    return {
      target_workspace_id: targetWorkspace.id,
      items: [{ type: assetType, id: assetId }],
    };
  }, [assetId, assetType, targetWorkspace?.id]);

  useEffect(() => {
    if (open) return;
    setTargetWorkspace(undefined);
    previewMutation.reset();
    moveMutation.reset();
    // Reset only on close; mutation objects change as their internal state changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const preview = previewMutation.data;
  const previewItems: WorkspaceAssetMovePreviewItem[] = preview?.items ?? [];
  const blockers: string[] = previewItems.flatMap(item => item.blockers ?? []);
  const warnings: string[] = previewItems.flatMap(item => item.warnings ?? []);
  const canSubmit =
    Boolean(request) && Boolean(preview) && preview?.movable && !previewMutation.isPending;

  const handleSubmit = async () => {
    if (!request || !canSubmit) return;
    await moveMutation.mutateAsync(request);
    onMoved?.();
    onOpenChange(false);
  };

  const handleTargetWorkspaceChange = (workspace: WorkspaceSelectorValue) => {
    setTargetWorkspace(workspace);
    previewMutation.mutate({
      target_workspace_id: workspace.id,
      items: [{ type: assetType, id: assetId }],
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground">
              {t('assetMove.targetWorkspace')}
            </label>
            <WorkspaceSelector
              value={targetWorkspace}
              onChange={handleTargetWorkspaceChange}
              placeholder={t('assetMove.targetWorkspacePlaceholder')}
            />
          </div>

          {previewMutation.isPending && (
            <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t('assetMove.previewing')}
            </div>
          )}

          {preview && previewItems[0] && (
            <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
              <span className="truncate text-muted-foreground">
                {previewItems[0].from_workspace?.name ||
                  previewItems[0].from_workspace?.id ||
                  t('assetMove.unknownWorkspace')}
              </span>
              <ArrowRight className="h-4 w-4 text-muted-foreground" />
              <span className="truncate font-medium">
                {previewItems[0].target_workspace?.name || targetWorkspace?.name}
              </span>
            </div>
          )}

          {blockers.length > 0 && (
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

          {warnings.length > 0 && blockers.length === 0 && (
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
          <Button variant="outline" onClick={() => onOpenChange(false)}>
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
