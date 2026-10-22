'use client';

import { useEffect, useState } from 'react';
import { usePathname } from 'next/navigation';
import { useIsMutating } from '@tanstack/react-query';
import {
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
import {
  DigitalHumanPublishVersionDialog,
  type DigitalHumanPublishVersionDetails,
} from '@/components/digital-human/digital-human-publish-version-dialog';
import { Badge } from '@/components/ui/badge';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { Button } from '@/components/ui/button';
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
  onOpenChat: () => void;
}

const EMPTY_PUBLISH_VERSION_DETAILS: DigitalHumanPublishVersionDetails = {
  name: '',
  description: '',
};

export function DigitalHumanHeader({ id, onOpenChat }: DigitalHumanHeaderProps) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const detailQuery = useDigitalHuman(id);
  const bindingsQuery = useDigitalHumanDraftBindings(id);
  const permissions = useAccountPermissions();
  const mutations = useDigitalHumanMutations(id);
  const draftEditor = useDigitalHumanDraftStore(state => state.editors[id]);
  const versionPreview = useDigitalHumanDraftStore(state => state.versionPreviews[id]);
  const [versionsOpen, setVersionsOpen] = useState(false);
  const [selectedVersionId, setSelectedVersionId] = useState('');
  const [publishOpen, setPublishOpen] = useState(false);
  const [publishVersionDetails, setPublishVersionDetails] =
    useState<DigitalHumanPublishVersionDetails>(EMPTY_PUBLISH_VERSION_DETAILS);
  const [basicInfoOpen, setBasicInfoOpen] = useState(false);
  const versionsQuery = useDigitalHumanVersions(id, versionsOpen);
  const restorePreviewQuery = useDigitalHumanVersionRestorePreview(
    id,
    selectedVersionId,
    versionsOpen && Boolean(selectedVersionId)
  );
  const detail = detailQuery.data?.data;
  // API keys and docs take effect at once; only the configuration and the
  // landing page edit the draft that publishing puts live.
  const isImmediatePage =
    pathname.startsWith(`/console/digital-humans/${id}/external/keys`) ||
    pathname.startsWith(`/console/digital-humans/${id}/external/docs`);
  const isDraftEditingPage =
    !isImmediatePage &&
    (pathname.startsWith(`/console/digital-humans/${id}/configuration`) ||
      pathname.startsWith(`/console/digital-humans/${id}/external`));
  // The server compares resource bindings too; the revision check is only a
  // fallback for responses that predate the field.
  const hasUnpublishedChanges = Boolean(
    detail &&
      (detail.has_unpublished_changes ??
        (!detail.published || detail.draft.revision !== detail.published.revision))
  );
  const savingBindings = useIsMutating({ mutationKey: DIGITAL_HUMAN_KEYS.replaceDraftBindings(id) }) > 0;
  const saveState = savingBindings ? 'saving' : (draftEditor?.saveState ?? 'saved');
  const isDirty = Boolean(draftEditor?.isDirty);
  const isVersionPreviewing = Boolean(versionPreview);
  const saveDotClassName =
    saveState === 'error'
      ? 'bg-destructive'
      : isDirty
        ? 'bg-yellow-400'
        : saveState === 'saved'
          ? 'bg-emerald-500'
          : 'bg-muted-foreground/40';
  const saveText = versionPreview
    ? t('versionHistory.previewingVersion', {
        version: versionPreview.version.version_number ?? '',
      })
    : draftEditor && !(saveState === 'saved' && !isDirty && hasUnpublishedChanges)
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
  const textIcon = getAgentTextIconDisplay(
    detail?.definition.icon_type,
    detail?.definition.icon,
    detail?.definition.name
  );

  const saveDraft = async () => {
    if (useDigitalHumanDraftStore.getState().versionPreviews[id]) return false;
    const editor = useDigitalHumanDraftStore.getState().editors[id];
    if (!editor) return true;
    return editor.saveNow();
  };

  const openPublishDialog = async () => {
    if (useDigitalHumanDraftStore.getState().versionPreviews[id]) return;
    const editor = useDigitalHumanDraftStore.getState().editors[id];
    if (editor && (editor.isDirty || editor.saveState === 'error')) {
      const saved = await editor.saveNow();
      if (!saved) return;
      await Promise.all([detailQuery.refetch(), bindingsQuery.refetch()]);
    }
    setPublishVersionDetails(EMPTY_PUBLISH_VERSION_DETAILS);
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
        name: publishVersionDetails.name.trim() || undefined,
        description: publishVersionDetails.description.trim() || undefined,
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
    useDigitalHumanDraftStore.getState().clearVersionPreview(id);
    useDigitalHumanDraftStore.getState().resetFromServer(id);
    setSelectedVersionId('');
    setVersionsOpen(false);
    toast.success(
      t('versionHistory.restored', {
        version: preview.version_number,
      })
    );
  };

  const clearVersionPreview = () => {
    useDigitalHumanDraftStore.getState().clearVersionPreview(id);
    setSelectedVersionId('');
  };

  const selectVersion = (versionId: string) => {
    const version = versionsQuery.data?.data.items.find(item => item.id === versionId);
    if (!version) return;
    const currentPreview = useDigitalHumanDraftStore.getState().versionPreviews[id];
    useDigitalHumanDraftStore.getState().setVersionPreview(id, {
      version,
      hadLocalDraftChanges: currentPreview?.hadLocalDraftChanges ?? isDirty,
    });
    setSelectedVersionId(versionId);
  };

  useEffect(() => {
    const preview = restorePreviewQuery.data?.data;
    const version = versionsQuery.data?.data.items.find(item => item.id === selectedVersionId);
    if (!preview || !version || preview.version_id !== version.id) return;
    const currentPreview = useDigitalHumanDraftStore.getState().versionPreviews[id];
    useDigitalHumanDraftStore.getState().setVersionPreview(id, {
      version,
      bindings: preview.bindings,
      hadLocalDraftChanges: currentPreview?.hadLocalDraftChanges ?? isDirty,
    });
  }, [id, isDirty, restorePreviewQuery.data?.data, selectedVersionId, versionsQuery.data?.data]);

  useEffect(() => {
    if (!selectedVersionId || !restorePreviewQuery.isError) return;
    useDigitalHumanDraftStore.getState().clearVersionPreview(id);
    setSelectedVersionId('');
    toast.error(t('versionHistory.previewFailed'));
  }, [id, restorePreviewQuery.isError, selectedVersionId, t]);

  useEffect(
    () => () => {
      useDigitalHumanDraftStore.getState().clearVersionPreview(id);
    },
    [id]
  );

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
          {isDraftEditingPage ? (
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

          {/* Autosave keeps the draft; a manual save is offered only when
              there is something it has not saved yet. */}
          {isDraftEditingPage && draftEditor?.canUpdate && (isDirty || saveState === 'error') ? (
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
                    disabled={draftEditor.isSaving || isVersionPreviewing}
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

          {detail?.published && isDraftEditingPage ? (
            <DigitalHumanVersionPopover
              open={versionsOpen}
              onOpenChange={nextOpen => {
                setVersionsOpen(nextOpen);
                if (!nextOpen) clearVersionPreview();
              }}
              isLoading={versionsQuery.isLoading}
              isLoadingPreview={restorePreviewQuery.isLoading}
              isRestoring={mutations.restoreVersion.isPending}
              hasUnpublishedChanges={hasUnpublishedChanges}
              hasLocalDraftChanges={versionPreview?.hadLocalDraftChanges ?? isDirty}
              isDraftSaving={Boolean(draftEditor?.isSaving)}
              draftUpdatedAt={detail?.draft.updated_at}
              currentVersionId={detail.published.id}
              versions={versionsQuery.data?.data.items ?? []}
              selectedVersionId={selectedVersionId}
              restorePreview={restorePreviewQuery.data?.data}
              canOpen={permissions.hasPermission('digital_human.publish')}
              canRestore={permissions.hasPermission('digital_human.publish')}
              onSelectVersion={selectVersion}
              onCancelPreview={clearVersionPreview}
              onRestore={() => void restoreVersion()}
            />
          ) : null}

          {detail?.published && permissions.hasPermission('digital_human.run') ? (
            <Button
              size="sm"
              className="inline-flex items-center gap-1.5 border border-emerald-600/30 bg-emerald-600 px-2 text-white shadow-none hover:border-emerald-700 hover:bg-emerald-700 sm:px-3.5"
              onClick={onOpenChat}
            >
              <Play className="size-4" fill="currentColor" />
              <span className="hidden font-semibold sm:inline">{t('header.chat')}</span>
            </Button>
          ) : null}

          {permissions.hasPermission('digital_human.publish') && isDraftEditingPage ? (
            <Button
              size="sm"
              className="flex items-center gap-1.5 border border-primary/25 bg-primary/10 px-2 text-primary shadow-none hover:border-primary/35 hover:bg-primary/15 sm:px-3.5"
              // Nothing to publish while the draft matches the live version.
              disabled={!detail || mutations.publish.isPending || isVersionPreviewing || (Boolean(detail?.published) && !hasUnpublishedChanges && !isDirty)}
              title={detail?.published && !hasUnpublishedChanges && !isDirty ? t('header.nothingToPublish') : undefined}
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

      <DigitalHumanPublishVersionDialog
        open={publishOpen}
        isUpdate={Boolean(detail?.published)}
        isSubmitting={mutations.publish.isPending}
        hasUnpublishedChanges={hasUnpublishedChanges}
        isLoadingBindings={bindingsQuery.isLoading}
        checks={publishChecks}
        value={publishVersionDetails}
        onOpenChange={setPublishOpen}
        onChange={setPublishVersionDetails}
        onConfirm={() => void publish()}
      />

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
  canOpen: boolean;
  canRestore: boolean;
  onSelectVersion: (versionId: string) => void;
  onCancelPreview: () => void;
  onRestore: () => void;
}

function formatVersionTime(version: DigitalHumanVersion): string {
  return new Date(version.published_at || version.created_at).toLocaleString(undefined, {
    hour12: false,
  });
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
  canOpen,
  canRestore,
  onSelectVersion,
  onCancelPreview,
  onRestore,
}: DigitalHumanVersionPopoverProps) {
  const t = useT('digitalHumans');
  // Overwriting unpublished work is confirmed in a centered dialog, which
  // stays visible however long the version list or short the window is.
  const [confirmOpen, setConfirmOpen] = useState(false);
  const selectedVersion = versions.find(version => version.id === selectedVersionId);
  const requiresDiscardConfirmation = Boolean(
    restorePreview?.will_discard_unpublished_changes || hasLocalDraftChanges
  );
  const selectedNumber = selectedVersion
    ? (selectedVersion.version_number ?? versions.length - versions.indexOf(selectedVersion))
    : undefined;
  const noChanges = Boolean(
    restorePreview &&
      !requiresDiscardConfirmation &&
      restorePreview.changed_fields.length === 0 &&
      restorePreview.binding_changes.length === 0
  );
  const restoreDisabled =
    !canRestore ||
    !selectedVersion ||
    !restorePreview?.impact_token ||
    isLoadingPreview ||
    isRestoring ||
    isDraftSaving ||
    noChanges;
  const requestRestore = () => {
    if (requiresDiscardConfirmation) setConfirmOpen(true);
    else onRestore();
  };

  useEffect(() => {
    setConfirmOpen(false);
  }, [selectedVersionId, restorePreview?.impact_token]);

  return (
    <Popover
      open={open && canOpen}
      onOpenChange={nextOpen => {
        // The confirmation dialog takes focus; that must not close the
        // panel and cancel the preview it is confirming.
        if (!nextOpen && confirmOpen) return;
        if (canOpen) onOpenChange(nextOpen);
      }}
    >
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
              disabled={!canOpen}
            >
              <History className="size-[18px]" />
            </Button>
          </TooltipTrigger>
        </PopoverTrigger>
        <TooltipContent>{t('header.versions')}</TooltipContent>
      </Tooltip>
      <PopoverContent
        align="end"
        sideOffset={10}
        collisionPadding={12}
        className="flex max-h-[var(--radix-popover-content-available-height)] w-[360px] flex-col p-0"
      >
        <PopoverHeader className="border-b px-4 py-3">
          <PopoverTitle>{t('versionHistory.title')}</PopoverTitle>
          <PopoverDescription>{t('versionHistory.description')}</PopoverDescription>
        </PopoverHeader>
        <div className="min-h-0 flex-1 overflow-y-auto">
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
                const publishedAt = formatVersionTime(version);
                const number = version.version_number ?? versions.length - index;
                // An unnamed version is called by its number; its time is on
                // the line below.
                const displayName =
                  version.name?.trim() || t('configuration.publishedVersion', { version: number });
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
                      <span className="truncate text-sm font-medium" title={displayName}>
                        {displayName}
                      </span>
                      <span className="flex shrink-0 items-center gap-1">
                        {current ? (
                          <Badge variant="subtle" className="gap-1">
                            <CheckCircle2 className="size-3" />
                            {t('versionHistory.current')}
                          </Badge>
                        ) : null}
                        {selected ? (
                          <Badge variant="outline">{t('versionHistory.previewing')}</Badge>
                        ) : null}
                      </span>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('versionHistory.publishedAt', { time: publishedAt })}
                      {version.name?.trim()
                        ? ` · ${t('configuration.publishedVersion', { version: number })}`
                        : ''}
                    </p>
                    {version.description?.trim() ? (
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {version.description.trim()}
                      </p>
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
                  {noChanges ? (
                    <p className="text-xs text-muted-foreground">{t('versionHistory.noChanges')}</p>
                  ) : null}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
        {/* Actions appear once a version is chosen; until then the footer
            says what to do instead of offering buttons that cannot work. */}
        <div className="shrink-0 border-t p-3">
          {selectedVersionId ? (
            <div className="flex items-center justify-between gap-2">
              <Button variant="outline" size="sm" onClick={onCancelPreview} disabled={isRestoring}>
                <X className="mr-1.5 size-4" />
                {t('versionHistory.cancelPreview')}
              </Button>
              <Button size="sm" onClick={requestRestore} disabled={restoreDisabled}>
                {isRestoring ? (
                  <Loader2 className="mr-1.5 size-4 animate-spin" />
                ) : (
                  <RotateCcw className="mr-1.5 size-4" />
                )}
                {t('versionHistory.restore')}
              </Button>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">{t('versionHistory.selectHint')}</p>
          )}
        </div>
      </PopoverContent>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        variant="warning"
        title={t('versionHistory.confirmTitle', { version: selectedNumber ?? '' })}
        description={
          <span className="block space-y-1">
            <span className="block">{t('versionHistory.confirmDiscardBody')}</span>
            {restorePreview ? (
              <span className="block">
                {t('versionHistory.impactSummary', {
                  fields: restorePreview.changed_fields.length,
                  resources: restorePreview.binding_changes.length,
                })}
              </span>
            ) : null}
            <span className="block">{t('versionHistory.confirmPublishNote')}</span>
          </span>
        }
        confirmText={t('versionHistory.confirmRestore')}
        loading={isRestoring}
        onConfirm={onRestore}
      />
    </Popover>
  );
}
