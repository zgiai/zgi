'use client';

import { useEffect, useState } from 'react';
import { usePathname } from 'next/navigation';
import {
  AlertTriangle,
  CheckCircle2,
  History,
  Loader2,
  Play,
  RotateCcw,
  Save,
  UploadCloud,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { DigitalHumanBasicInfoDialog } from '@/components/digital-human/digital-human-basic-info-dialog';
import { useDigitalHumanDraftStore } from '@/components/digital-human/digital-human-draft-store';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  useDigitalHuman,
  useDigitalHumanDraftBindings,
  useDigitalHumanMutations,
  useDigitalHumanVersionRestorePreview,
  useDigitalHumanVersions,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { getAgentTextIconDisplay } from '@/components/agents/agent-runtime/utils';
import type {
  DigitalHumanVersion,
  DigitalHumanVersionRestorePreview,
} from '@/services/types/digital-human';

interface DigitalHumanHeaderProps {
  id: string;
  onOpenTest: () => void;
}

export function DigitalHumanHeader({ id, onOpenTest }: DigitalHumanHeaderProps) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const detailQuery = useDigitalHuman(id);
  const bindingsQuery = useDigitalHumanDraftBindings(id);
  const permissions = useAccountPermissions();
  const mutations = useDigitalHumanMutations(id);
  const draftEditor = useDigitalHumanDraftStore(state => state.editors[id]);
  const [versionsOpen, setVersionsOpen] = useState(false);
  const [selectedVersionId, setSelectedVersionId] = useState('');
  const [publishOpen, setPublishOpen] = useState(false);
  const [basicInfoOpen, setBasicInfoOpen] = useState(false);
  const versionsQuery = useDigitalHumanVersions(id, versionsOpen);
  const restorePreviewQuery = useDigitalHumanVersionRestorePreview(
    id,
    selectedVersionId,
    versionsOpen && Boolean(selectedVersionId)
  );
  const detail = detailQuery.data?.data;
  const isConfigurationPage = pathname.startsWith(`/console/digital-humans/${id}/configuration`);
  const hasUnpublishedChanges = Boolean(
    detail && (!detail.published || detail.draft.revision !== detail.published.revision)
  );
  const saveState = draftEditor?.saveState ?? 'saved';
  const isDirty = Boolean(draftEditor?.isDirty);
  const saveDotClassName =
    saveState === 'error'
      ? 'bg-destructive'
      : isDirty
        ? 'bg-yellow-400'
        : saveState === 'saved'
          ? 'bg-emerald-500'
          : 'bg-muted-foreground/40';
  const saveText = draftEditor
    ? t(`draftPersistence.${saveState}`)
    : hasUnpublishedChanges
      ? t('header.draftSavedPending')
      : t('header.draftSaved');
  const publishChecks = detail
    ? [
        {
          label: t('configuration.preflight.identity'),
          ready: Boolean(detail.draft.identity_name.trim() && detail.draft.persona.trim()),
          blocking: true,
        },
        {
          label: t('configuration.preflight.model'),
          ready: Boolean(detail.draft.default_model.trim()),
          blocking: true,
        },
        {
          label: t('configuration.preflight.bindings'),
          ready: Boolean(bindingsQuery.data?.data?.items.length),
          blocking: false,
        },
      ]
    : [];
  const blockingIssues = publishChecks.filter(check => check.blocking && !check.ready);
  const warningCount = publishChecks.filter(check => !check.blocking && !check.ready).length;
  const textIcon = getAgentTextIconDisplay(
    detail?.definition.icon_type,
    detail?.definition.icon,
    detail?.definition.name
  );

  const saveDraft = async () => {
    const editor = useDigitalHumanDraftStore.getState().editors[id];
    if (!editor) return true;
    return editor.saveNow();
  };

  const openPublishDialog = async () => {
    const editor = useDigitalHumanDraftStore.getState().editors[id];
    if (editor && (editor.isDirty || editor.saveState === 'error')) {
      const saved = await editor.saveNow();
      if (!saved) return;
      await Promise.all([detailQuery.refetch(), bindingsQuery.refetch()]);
    }
    setPublishOpen(true);
  };

  const publish = async () => {
    const current = detailQuery.data?.data;
    if (!current) return;
    await mutations.publish.mutateAsync({
      digitalHumanId: id,
      request: {
        expected_revision: current.definition.revision,
        expected_draft_lock_version: current.draft.lock_version,
      },
    });
    setPublishOpen(false);
    toast.success(t('configuration.publishedSuccess'));
  };

  const restoreVersion = async () => {
    const current = detailQuery.data?.data;
    const preview = restorePreviewQuery.data?.data;
    if (!current || !preview || !selectedVersionId) return;
    await mutations.restoreVersion.mutateAsync({
      digitalHumanId: id,
      request: {
        version_id: selectedVersionId,
        expected_revision: current.definition.revision,
        expected_draft_lock_version: current.draft.lock_version,
        impact_token: preview.impact_token,
      },
    });
    useDigitalHumanDraftStore.getState().resetFromServer(id);
    setSelectedVersionId('');
    setVersionsOpen(false);
    toast.success(
      t('versionHistory.restored', {
        version: preview.version_number,
      })
    );
  };

  return (
    <>
      <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b bg-background px-3 sm:px-4">
        <div className="flex min-w-0 items-center">
          <div
            role={permissions.hasPermission('digital_human.update') ? 'button' : undefined}
            tabIndex={permissions.hasPermission('digital_human.update') ? 0 : undefined}
            className={cn(
              '-m-1 flex min-w-0 items-center gap-3 rounded-lg p-1 text-left outline-none transition-colors',
              permissions.hasPermission('digital_human.update') &&
                'cursor-pointer hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2'
            )}
            onClick={
              permissions.hasPermission('digital_human.update')
                ? () => setBasicInfoOpen(true)
                : undefined
            }
            onKeyDown={
              permissions.hasPermission('digital_human.update')
                ? event => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault();
                      setBasicInfoOpen(true);
                    }
                  }
                : undefined
            }
            aria-label={
              permissions.hasPermission('digital_human.update') ? t('basicInfo.edit') : undefined
            }
          >
            <div
              className="flex size-9 shrink-0 items-center justify-center rounded-lg text-sm font-semibold"
              style={
                detail?.definition.icon_type === 'image' &&
                (detail.definition.icon_url || detail.definition.avatar)
                  ? undefined
                  : { backgroundColor: textIcon.background }
              }
            >
              {detail?.definition.icon_type === 'image' &&
              (detail.definition.icon_url || detail.definition.avatar) ? (
                <img
                  src={detail.definition.icon_url || detail.definition.avatar}
                  alt=""
                  className="size-full rounded-lg object-cover"
                />
              ) : (
                <span className="text-white">{textIcon.text}</span>
              )}
            </div>
            <div className="min-w-0 max-w-[360px]">
              <div className="flex min-w-0 items-center gap-2">
                <h1 className="truncate text-sm font-semibold">
                  {detail?.definition.name ?? t('title')}
                </h1>
                {detail?.published ? (
                  <Badge variant="subtle" className="hidden shrink-0 sm:inline-flex">
                    {t('header.publishedVersion', {
                      version: detail.published.version_number ?? 1,
                    })}
                  </Badge>
                ) : null}
              </div>
              {detail?.definition.description ? (
                <div className="hidden truncate text-xs text-muted-foreground lg:block">
                  {detail.definition.description}
                </div>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-1.5">
          {isConfigurationPage ? (
            <div
              className={cn(
                'hidden items-center gap-1.5 text-xs text-muted-foreground xl:flex',
                saveState === 'error' && 'text-destructive'
              )}
            >
              {saveState === 'saving' ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : saveState === 'saved' ? (
                <CheckCircle2 className="size-3.5 text-success" />
              ) : null}
              {saveText}
            </div>
          ) : null}

          {isConfigurationPage && draftEditor?.canUpdate ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="relative">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    interactive="subtle"
                    aria-label={t('configuration.save')}
                    disabled={draftEditor.isSaving}
                    onClick={() => void saveDraft()}
                  >
                    {draftEditor.isSaving ? (
                      <Loader2 className="size-[18px] animate-spin" />
                    ) : (
                      <Save className="size-[18px]" />
                    )}
                    <span
                      className={cn(
                        'absolute right-1 top-1 size-2 rounded-full border-2 border-background',
                        saveDotClassName
                      )}
                    />
                  </Button>
                </div>
              </TooltipTrigger>
              <TooltipContent>{t('configuration.save')}</TooltipContent>
            </Tooltip>
          ) : null}

          {detail?.published ? (
            <DigitalHumanVersionPopover
              open={versionsOpen}
              onOpenChange={nextOpen => {
                setVersionsOpen(nextOpen);
                if (!nextOpen) setSelectedVersionId('');
              }}
              isLoading={versionsQuery.isLoading}
              isLoadingPreview={restorePreviewQuery.isLoading}
              isRestoring={mutations.restoreVersion.isPending}
              hasUnpublishedChanges={hasUnpublishedChanges}
              hasLocalDraftChanges={isDirty}
              isDraftSaving={Boolean(draftEditor?.isSaving)}
              draftUpdatedAt={detail?.draft.updated_at}
              currentVersionId={detail.published.id}
              versions={versionsQuery.data?.data.items ?? []}
              selectedVersionId={selectedVersionId}
              restorePreview={restorePreviewQuery.data?.data}
              canRestore={permissions.hasPermission('digital_human.update')}
              onSelectVersion={setSelectedVersionId}
              onCancelPreview={() => setSelectedVersionId('')}
              onRestore={() => void restoreVersion()}
            />
          ) : null}

          {detail?.published && permissions.hasPermission('digital_human.run') ? (
            <Button
              size="sm"
              className="inline-flex items-center gap-1.5 border border-emerald-600/30 bg-emerald-600 px-2 text-white shadow-none hover:border-emerald-700 hover:bg-emerald-700 sm:px-3.5"
              onClick={onOpenTest}
            >
              <Play className="size-4" fill="currentColor" />
              <span className="hidden font-semibold sm:inline">{t('header.test')}</span>
            </Button>
          ) : null}

          {permissions.hasPermission('digital_human.publish') ? (
            <Button
              size="sm"
              className="flex items-center gap-1.5 border border-primary/25 bg-primary/10 px-2 text-primary shadow-none hover:border-primary/35 hover:bg-primary/15 sm:px-3.5"
              disabled={!detail || mutations.publish.isPending}
              onClick={() => void openPublishDialog()}
            >
              {mutations.publish.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <UploadCloud className="size-4" />
              )}
              <span className="hidden font-semibold sm:inline">
                {detail?.published ? t('header.publishChanges') : t('header.publish')}
              </span>
            </Button>
          ) : null}
        </div>
      </header>

      <Dialog open={publishOpen} onOpenChange={setPublishOpen}>
        <DialogContent size="md" className="p-0 text-left">
          <DialogHeader>
            <div className="flex items-start gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <UploadCloud className="size-5" />
              </div>
              <div className="min-w-0">
                <DialogTitle>
                  {detail?.published
                    ? t('publishDialog.updateTitle')
                    : t('publishDialog.publishTitle')}
                </DialogTitle>
                <DialogDescription className="mt-1.5">
                  {t('publishDialog.description')}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="rounded-lg border bg-muted/20 p-3">
              <div className="mb-2 text-sm font-medium">{t('configuration.preflight.title')}</div>
              <div className="space-y-2">
                {publishChecks.map(check => (
                  <div key={check.label} className="flex items-center gap-2 text-sm">
                    {check.ready ? (
                      <CheckCircle2 className="size-4 shrink-0 text-emerald-600" />
                    ) : (
                      <AlertTriangle
                        className={cn(
                          'size-4 shrink-0',
                          check.blocking ? 'text-destructive' : 'text-amber-500'
                        )}
                      />
                    )}
                    <span className={check.ready ? 'text-foreground' : 'text-muted-foreground'}>
                      {check.label}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            {warningCount > 0 && blockingIssues.length === 0 ? (
              <p className="text-xs leading-5 text-amber-600 dark:text-amber-400">
                {t('configuration.preflight.warning', { count: warningCount })}
              </p>
            ) : null}
            <p className="text-xs leading-5 text-muted-foreground">
              {t('publishDialog.immutableHint')}
            </p>
          </DialogBody>
          <DialogFooter className="border-t bg-muted/20">
            <Button
              type="button"
              variant="ghost"
              disabled={mutations.publish.isPending}
              onClick={() => setPublishOpen(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              loading={mutations.publish.isPending}
              disabled={
                !hasUnpublishedChanges ||
                mutations.publish.isPending ||
                bindingsQuery.isLoading ||
                blockingIssues.length > 0
              }
              onClick={() => void publish()}
            >
              {detail?.published ? t('configuration.publishChanges') : t('configuration.publish')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <DigitalHumanBasicInfoDialog
        definition={detail?.definition}
        open={basicInfoOpen}
        onOpenChange={setBasicInfoOpen}
      />
    </>
  );
}

interface DigitalHumanVersionPopoverProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isLoading: boolean;
  isLoadingPreview: boolean;
  isRestoring: boolean;
  hasUnpublishedChanges: boolean;
  hasLocalDraftChanges: boolean;
  isDraftSaving: boolean;
  draftUpdatedAt?: string;
  currentVersionId?: string;
  versions: DigitalHumanVersion[];
  selectedVersionId: string;
  restorePreview?: DigitalHumanVersionRestorePreview;
  canRestore: boolean;
  onSelectVersion: (versionId: string) => void;
  onCancelPreview: () => void;
  onRestore: () => void;
}

function DigitalHumanVersionPopover({
  open,
  onOpenChange,
  isLoading,
  isLoadingPreview,
  isRestoring,
  hasUnpublishedChanges,
  hasLocalDraftChanges,
  isDraftSaving,
  draftUpdatedAt,
  currentVersionId,
  versions,
  selectedVersionId,
  restorePreview,
  canRestore,
  onSelectVersion,
  onCancelPreview,
  onRestore,
}: DigitalHumanVersionPopoverProps) {
  const t = useT('digitalHumans');
  const [discardConfirmed, setDiscardConfirmed] = useState(false);
  const selectedVersion = versions.find(version => version.id === selectedVersionId);
  const requiresDiscardConfirmation = Boolean(
    restorePreview?.will_discard_unpublished_changes || hasLocalDraftChanges
  );

  useEffect(() => {
    setDiscardConfirmed(false);
  }, [selectedVersionId, restorePreview?.impact_token]);

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <Tooltip>
        <PopoverTrigger asChild>
          <TooltipTrigger asChild>
            <Button
              type="button"
              isIcon
              variant="ghost"
              size="sm"
              interactive="subtle"
              aria-label={t('header.versions')}
            >
              <History className="size-[18px]" />
            </Button>
          </TooltipTrigger>
        </PopoverTrigger>
        <TooltipContent>{t('header.versions')}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" sideOffset={10} className="w-[360px] p-0">
        <PopoverHeader className="border-b px-4 py-3">
          <PopoverTitle>{t('versionHistory.title')}</PopoverTitle>
          <PopoverDescription>{t('versionHistory.description')}</PopoverDescription>
        </PopoverHeader>
        <div className="max-h-[min(460px,calc(100vh-180px))] overflow-y-auto">
          <div className="m-4 mb-0 rounded-lg border border-primary/20 bg-primary/5 p-3">
            <div className="flex items-center justify-between gap-2">
              <span className="text-sm font-medium">{t('configuration.currentDraft')}</span>
              <Badge variant={hasUnpublishedChanges ? 'info' : 'success'}>
                {hasUnpublishedChanges
                  ? t('configuration.pendingPublish')
                  : t('configuration.inSync')}
              </Badge>
            </div>
            {draftUpdatedAt ? (
              <p className="mt-1 text-xs text-muted-foreground">
                {t('configuration.draftUpdatedAt', {
                  date: new Date(draftUpdatedAt).toLocaleString(),
                })}
              </p>
            ) : null}
          </div>

          {isLoading ? (
            <div className="space-y-3 p-4 pl-7">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : versions.length === 0 ? (
            <div className="m-4 rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
              {t('versionHistory.empty')}
            </div>
          ) : (
            <div className="space-y-3 p-4 pl-7">
              {versions.map((version, index) => {
                const current = version.id === currentVersionId;
                const selected = version.id === selectedVersionId;
                return (
                  <button
                    key={version.id}
                    type="button"
                    className={cn(
                      'relative w-full rounded-lg border bg-background p-3 text-left transition-colors hover:border-primary/50',
                      selected && 'border-primary bg-primary/5'
                    )}
                    onClick={() => onSelectVersion(version.id)}
                  >
                    <span className="absolute -left-[17px] top-4 size-2 rounded-full bg-border" />
                    {index < versions.length - 1 ? (
                      <span className="absolute -left-[14px] top-6 h-[calc(100%+12px)] w-px bg-border" />
                    ) : null}
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-medium">
                        {t('configuration.publishedVersion', {
                          version: version.version_number ?? versions.length - index,
                        })}
                      </span>
                      {current ? (
                        <Badge variant="subtle" className="gap-1">
                          <CheckCircle2 className="size-3" />
                          {t('versionHistory.current')}
                        </Badge>
                      ) : null}
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {version.published_at
                        ? t('configuration.publishedAt', {
                            date: new Date(version.published_at).toLocaleString(),
                          })
                        : t('configuration.publishedAvailable')}
                    </p>
                    {selected ? (
                      <Badge variant="outline" className="mt-2">
                        {t('versionHistory.previewing')}
                      </Badge>
                    ) : null}
                  </button>
                );
              })}
            </div>
          )}

          {selectedVersionId ? (
            <div className="border-t px-4 py-3">
              {isLoadingPreview ? (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  {t('versionHistory.loadingImpact')}
                </div>
              ) : restorePreview ? (
                <div className="space-y-3">
                  <div>
                    <div className="text-sm font-medium">{t('versionHistory.impactTitle')}</div>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {t('versionHistory.impactSummary', {
                        fields: restorePreview.changed_fields.length,
                        resources: restorePreview.binding_changes.length,
                      })}
                    </p>
                  </div>
                  {restorePreview.binding_changes.length > 0 ? (
                    <div className="max-h-28 space-y-1 overflow-y-auto rounded-md border p-2">
                      {restorePreview.binding_changes.map((item, index) => (
                        <div
                          key={`${item.change}:${item.type}:${item.parent_resource_id ?? ''}:${item.resource_id}:${index}`}
                          className="flex items-center justify-between gap-2 text-xs"
                        >
                          <span className="truncate text-muted-foreground">
                            {item.display_name || item.resource_id}
                          </span>
                          <span className="shrink-0 text-muted-foreground">
                            {t(`versionHistory.bindingChange.${item.change}`)}
                          </span>
                        </div>
                      ))}
                    </div>
                  ) : null}
                  {requiresDiscardConfirmation ? (
                    <label className="flex cursor-pointer items-start gap-2 text-xs leading-5">
                      <Checkbox
                        checked={discardConfirmed}
                        onCheckedChange={checked => setDiscardConfirmed(checked === true)}
                      />
                      <span>{t('versionHistory.confirmDiscard')}</span>
                    </label>
                  ) : (
                    <p className="text-xs text-muted-foreground">{t('versionHistory.noChanges')}</p>
                  )}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
        <div className="flex items-center justify-between gap-2 border-t p-3">
          <Button
            variant="outline"
            size="sm"
            onClick={onCancelPreview}
            disabled={!selectedVersionId || isRestoring}
          >
            <X className="mr-1.5 size-4" />
            {t('versionHistory.cancelPreview')}
          </Button>
          <Button
            size="sm"
            onClick={onRestore}
            disabled={
              !canRestore ||
              !selectedVersion ||
              !restorePreview?.impact_token ||
              isLoadingPreview ||
              isRestoring ||
              isDraftSaving ||
              (requiresDiscardConfirmation && !discardConfirmed)
            }
          >
            {isRestoring ? (
              <Loader2 className="mr-1.5 size-4 animate-spin" />
            ) : (
              <RotateCcw className="mr-1.5 size-4" />
            )}
            {t('versionHistory.restore')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
