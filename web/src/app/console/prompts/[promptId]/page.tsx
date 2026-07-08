'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useRouter, useSearchParams } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  Copy,
  FlaskConical,
  GitCompare,
  Link2,
  PencilLine,
  Save,
  Settings2,
  ShieldAlert,
  Share2,
  WandSparkles,
} from 'lucide-react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import {
  usePrompt,
  useCreatePrompt,
  useCreatePromptVersion,
  useUpdatePrompt,
  usePromptUsage,
  useSetPromptLabels,
} from '@/hooks/prompt/use-prompts';
import { PromptVersionDialog } from '@/components/prompts/prompt-version-dialog';
import { PromptOptimizationHistory } from '@/components/prompts/prompt-optimization-history';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { PromptVersionCompareDialog } from '@/components/prompts/prompt-version-compare-dialog';
import {
  promptLocaleLabelKey,
  promptTypeLabelKey,
} from '@/components/prompts/prompt-display-labels';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { toast } from 'sonner';
import { findTemplatesByPromptId } from '@/components/agents/templates/template-manifest';
import {
  getTemplateCopy,
  type TemplateTranslator,
} from '@/components/agents/templates/template-labels';
import type {
  CreatePromptRequest,
  PromptOptimizationRun,
  PromptUsageReference,
  PromptVersion,
} from '@/services/types/prompt';

function stringifyPromptContent(content: PromptVersion['content']) {
  return typeof content === 'string' ? content : JSON.stringify(content, null, 2);
}

export default function PromptDetailPage() {
  const t = useT('prompts');
  const rootT = useT();
  const params = useParams<{ promptId: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const promptId = params?.promptId ?? '';
  const templateT = rootT as unknown as TemplateTranslator;
  const currentWorkspace = useCurrentWorkspace();
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('agent.view');
  const canManage = hasPermission('agent.manage');
  const { prompt, isLoading } = usePrompt(promptId, canView);
  const {
    usage,
    isLoading: isUsageLoading,
    error: usageError,
  } = usePromptUsage(promptId, canView && Boolean(prompt));
  const targetWorkspaceId = prompt?.workspace_id ?? '';
  const createPrompt = useCreatePrompt();
  const createVersion = useCreatePromptVersion(promptId);
  const updatePrompt = useUpdatePrompt(promptId);
  const setPromptLabels = useSetPromptLabels(promptId);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [optimizerOpen, setOptimizerOpen] = useState(false);
  const [compareOpen, setCompareOpen] = useState(false);
  const [shareConfirmOpen, setShareConfirmOpen] = useState(false);
  const [publishConfirmOpen, setPublishConfirmOpen] = useState(false);
  const [publishTargetVersion, setPublishTargetVersion] = useState<PromptVersion | null>(null);
  const [metadataOpen, setMetadataOpen] = useState(false);
  const [selectedHistoryVersion, setSelectedHistoryVersion] = useState<number | null>(null);
  const [nameDraft, setNameDraft] = useState('');
  const [descriptionDraft, setDescriptionDraft] = useState('');
  const [localeDraft, setLocaleDraft] = useState('zh-Hans');
  const [categoryDraft, setCategoryDraft] = useState('');
  const [tagsDraft, setTagsDraft] = useState('');
  const [sourceDraft, setSourceDraft] = useState<'personal' | 'workspace'>('personal');
  const [optimizerPresetRun, setOptimizerPresetRun] = useState<PromptOptimizationRun | null>(null);
  const appliedDetailActionRef = useRef<string | null>(null);

  const versions = useMemo(() => prompt?.versions ?? [], [prompt?.versions]);
  const currentVersion = versions[0] ?? null;
  const previousVersions = useMemo(() => versions.slice(1), [versions]);
  const hasSingleVersion = versions.length <= 1;
  const onlineVersion = versions.find(version => version.labels.includes('production')) ?? null;
  const canManagePromptDetails = Boolean(prompt && canManage && prompt.source !== 'official');
  const relatedTemplates = useMemo(() => findTemplatesByPromptId(promptId), [promptId]);
  const latestVersionText = useMemo(() => {
    const latest = versions[0];
    if (!latest) return '';
    return typeof latest.content === 'string'
      ? latest.content
      : JSON.stringify(latest.content, null, 2);
  }, [versions]);
  const latestReferenceCount = useMemo(() => {
    return (
      usage?.references?.filter(reference => {
        return reference.reference_mode === 'label' && reference.label?.toLowerCase() === 'latest';
      }).length ?? 0
    );
  }, [usage?.references]);
  const productionReferenceCount = useMemo(() => {
    return (
      usage?.references?.filter(reference => {
        return (
          reference.reference_mode === 'label' && reference.label?.toLowerCase() === 'production'
        );
      }).length ?? 0
    );
  }, [usage?.references]);
  const publishImpactDescription = useMemo(() => {
    if (isUsageLoading || usageError) {
      return t('detail.publishOnlineConfirmImpactUnknown');
    }
    if (productionReferenceCount > 0) {
      return t('detail.publishOnlineConfirmImpactWithReferences', {
        count: productionReferenceCount,
      });
    }
    return t('detail.publishOnlineConfirmImpactNoReferences');
  }, [isUsageLoading, productionReferenceCount, t, usageError]);
  const visibleReferences = useMemo(() => {
    const references = usage?.references ?? [];
    return [...references]
      .sort((left, right) => {
        const leftLatest =
          left.reference_mode === 'label' && left.label?.toLowerCase() === 'latest';
        const rightLatest =
          right.reference_mode === 'label' && right.label?.toLowerCase() === 'latest';
        if (leftLatest !== rightLatest) return leftLatest ? -1 : 1;
        return new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime();
      })
      .slice(0, 3);
  }, [usage?.references]);
  const hiddenReferenceCount = Math.max(
    (usage?.references?.length ?? 0) - visibleReferences.length,
    0
  );
  const legacyLinkedNodes = usage?.linked_nodes_count ?? 0;
  const legacyTotalRuns = usage?.total_run_count ?? 0;
  const hasLegacyEvidence = Boolean(
    usage &&
      (legacyLinkedNodes > 0 ||
        legacyTotalRuns > 0 ||
        visibleReferences.length > 0 ||
        latestReferenceCount > 0 ||
        usage.last_run_at)
  );
  const formatReferenceTarget = (reference: PromptUsageReference) => {
    if (reference.reference_mode === 'version' && reference.version) {
      return `v${reference.version}`;
    }
    if (!reference.label) {
      return t('usage.referenceModes.managed');
    }
    const normalized = reference.label.toLowerCase();
    if (normalized === 'production') return t('picker.releaseLabels.production');
    if (normalized === 'latest') return t('picker.releaseLabels.latest');
    if (normalized === 'staging') return t('picker.releaseLabels.staging');
    if (normalized === 'gray-a') return t('picker.releaseLabels.grayA');
    if (normalized === 'gray-b') return t('picker.releaseLabels.grayB');
    return reference.label;
  };
  const releaseImpactDescription = useMemo(() => {
    if (!currentVersion) return '';
    if (!onlineVersion) return t('detail.releaseImpactUnset');
    if (onlineVersion.version === currentVersion.version) {
      return t('detail.releaseImpactCurrent', { version: `v${currentVersion.version}` });
    }
    return t('detail.releaseImpactPending', {
      latestVersion: `v${currentVersion.version}`,
      onlineVersion: `v${onlineVersion.version}`,
    });
  }, [currentVersion, onlineVersion, t]);
  const assetHealth = useMemo(() => {
    if (!hasLegacyEvidence) return null;

    if (onlineVersion && latestReferenceCount > 0) {
      return {
        kind: 'risk' as const,
        title: t('detail.assetHealthRiskTitle'),
        description: t('detail.assetHealthRiskDescription', {
          count: latestReferenceCount,
          onlineVersion: `v${onlineVersion.version}`,
        }),
        badge: t('detail.assetHealthRiskBadge'),
      };
    }

    if (legacyTotalRuns > 0 || usage?.last_run_at) {
      return {
        kind: 'active' as const,
        title: t('detail.assetHealthActiveTitle'),
        description: t('detail.assetHealthActiveDescription'),
        badge: t('detail.assetHealthActiveBadge'),
      };
    }

    if (legacyLinkedNodes > 0) {
      return {
        kind: 'linked' as const,
        title: t('detail.assetHealthLinkedTitle'),
        description: t('detail.assetHealthLinkedDescription'),
        badge: t('detail.assetHealthLinkedBadge'),
      };
    }

    return null;
  }, [
    hasLegacyEvidence,
    latestReferenceCount,
    legacyLinkedNodes,
    legacyTotalRuns,
    onlineVersion,
    t,
    usage?.last_run_at,
  ]);
  const selectedPreviousVersion = useMemo(() => {
    return (
      previousVersions.find(version => version.version === selectedHistoryVersion) ??
      previousVersions[0] ??
      null
    );
  }, [previousVersions, selectedHistoryVersion]);
  const nextVersionNumber = currentVersion ? currentVersion.version + 1 : versions.length + 1;

  const displayVersionLabel = (label: string) => {
    const normalized = label.toLowerCase();
    if (normalized === 'production') return t('picker.releaseLabels.production');
    if (normalized === 'latest') return t('picker.releaseLabels.latest');
    if (normalized === 'staging') return t('picker.releaseLabels.staging');
    if (normalized === 'gray-a') return t('picker.releaseLabels.grayA');
    if (normalized === 'gray-b') return t('picker.releaseLabels.grayB');
    return label;
  };

  useEffect(() => {
    if (!prompt) return;
    setNameDraft(prompt.name);
    setDescriptionDraft(prompt.description ?? '');
    setLocaleDraft(prompt.locale);
    setCategoryDraft(prompt.category ?? '');
    setTagsDraft(prompt.tags.join(', '));
    setSourceDraft(prompt.source === 'official' ? 'personal' : prompt.source);
  }, [prompt]);

  useEffect(() => {
    if (!previousVersions.length) {
      if (selectedHistoryVersion !== null) {
        setSelectedHistoryVersion(null);
      }
      return;
    }
    if (!previousVersions.some(version => version.version === selectedHistoryVersion)) {
      setSelectedHistoryVersion(previousVersions[0].version);
    }
  }, [previousVersions, selectedHistoryVersion]);

  useEffect(() => {
    if (!prompt) return;
    const action = searchParams.get('action');
    if (action !== 'edit' && action !== 'optimize') return;
    const actionKey = `${prompt.id}:${action}`;
    if (appliedDetailActionRef.current === actionKey) return;

    if (action === 'optimize') {
      setOptimizerPresetRun(null);
      setOptimizerOpen(true);
      appliedDetailActionRef.current = actionKey;
      return;
    }

    if (canManagePromptDetails) {
      setDialogOpen(true);
      appliedDetailActionRef.current = actionKey;
    }
  }, [canManagePromptDetails, prompt, searchParams]);

  const handleShareToWorkspace = () => {
    if (!prompt || prompt.source !== 'personal') return;
    updatePrompt.mutate({
      data: { source: 'workspace' },
      successMessage: t('messages.shareToWorkspaceSuccess'),
      errorMessage: t('messages.shareToWorkspaceFailed'),
    });
  };

  const handleCreatePersonalCopy = async (
    contentOverride?: string,
    options: { optimized?: boolean } = {}
  ) => {
    if (!prompt || !currentVersion || !currentWorkspace?.id) {
      toast.error(t('messages.copyAsPersonalFailed'));
      throw new Error('A workspace is required to copy this prompt.');
    }

    const payload: CreatePromptRequest = {
      workspace_id: currentWorkspace.id,
      source: 'personal',
      name: options.optimized
        ? t('detail.optimizedCopyName', { name: prompt.name })
        : t('detail.personalCopyName', { name: prompt.name }),
      description: prompt.description ?? null,
      locale: prompt.locale,
      category: prompt.category ?? null,
      tags: prompt.tags,
      initial_version: {
        prompt_type: contentOverride ? 'text' : currentVersion.prompt_type,
        content: contentOverride ?? currentVersion.content,
        labels: [],
        commit_message: options.optimized
          ? t('detail.optimizedCopyCommitMessage')
          : t('detail.personalCopyCommitMessage'),
      },
    };

    const created = await createPrompt.mutateAsync({
      data: payload,
      successMessage: options.optimized
        ? t('messages.optimizedCopyAsPersonalSuccess')
        : t('messages.copyAsPersonalSuccess'),
      errorMessage: t('messages.copyAsPersonalFailed'),
    });
    const createdId = created.data?.id;
    if (createdId) {
      router.push(`/console/prompts/${createdId}`);
    }
  };

  const handleApplyOptimizedPrompt = async (payload: { text: string }) => {
    if (!prompt || !currentVersion) return;
    if (prompt.source === 'official') {
      await handleCreatePersonalCopy(payload.text, { optimized: true });
      return;
    }

    await createVersion.mutateAsync({
      prompt_type: 'text',
      content: payload.text,
      labels: [],
      commit_message: t('detail.optimizedVersionCommitMessage'),
    });
  };

  const openPublishConfirm = (version: PromptVersion) => {
    setPublishTargetVersion(version);
    setPublishConfirmOpen(true);
  };

  const handleSetVersionOnline = () => {
    if (!publishTargetVersion || !canManagePromptDetails) return;
    const labels = Array.from(
      new Set([...publishTargetVersion.labels.filter(label => label !== 'latest'), 'production'])
    );
    setPromptLabels.mutate({
      version: publishTargetVersion.version,
      labels,
    });
  };

  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('states.accessDeniedTitle')}</h2>
        <p className="text-muted-foreground max-w-md">{t('states.accessDeniedDescription')}</p>
      </div>
    );
  }

  return (
    <WorkspaceMismatchGuard isLoading={isLoading} targetWorkspaceId={targetWorkspaceId}>
      <>
        <div className="p-4 sm:p-6 lg:p-8 space-y-5 flex flex-col h-full overflow-y-auto">
          <div className="rounded-xl border bg-background p-4 sm:p-5 space-y-4">
            <div className="flex items-start justify-between gap-4">
              <div className="space-y-2 min-w-0">
                <Link
                  href="/console/prompts"
                  className="text-sm text-muted-foreground inline-flex items-center gap-1 hover:text-foreground"
                >
                  <ArrowLeft className="h-4 w-4" />
                  {t('actions.back')}
                </Link>
                <div className="flex items-center gap-2 flex-wrap">
                  <h1 className="text-2xl font-semibold">{prompt?.name ?? t('states.loading')}</h1>
                  {prompt ? (
                    <Badge variant="secondary">{t(`sources.${prompt.source}`)}</Badge>
                  ) : null}
                  {prompt ? (
                    <Badge variant="outline">{t(promptLocaleLabelKey(prompt.locale))}</Badge>
                  ) : null}
                </div>
                {prompt?.description ? (
                  <p className="text-sm text-muted-foreground">{prompt.description}</p>
                ) : null}
              </div>
              <div className="flex items-center gap-2 flex-wrap justify-end">
                <Button
                  variant="outline"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(window.location.href);
                      toast.success(t('messages.shareCopied'));
                    } catch {
                      toast.error(t('messages.shareCopyFailed'));
                    }
                  }}
                  disabled={!prompt}
                >
                  <Copy className="h-4 w-4" />
                  {t('actions.copyLink')}
                </Button>
                <Button asChild disabled={!prompt}>
                  <Link href={`/console/prompts?tab=playground&promptId=${promptId}`}>
                    <FlaskConical className="h-4 w-4" />
                    {t('actions.testInPlayground')}
                  </Link>
                </Button>
                {prompt?.source === 'official' && canManage ? (
                  <Button
                    variant="outline"
                    onClick={() => {
                      void handleCreatePersonalCopy().catch(() => undefined);
                    }}
                    disabled={!currentVersion || !currentWorkspace?.id || createPrompt.isPending}
                  >
                    <Copy className="h-4 w-4" />
                    {t('actions.copyAsPersonal')}
                  </Button>
                ) : null}
                <Button
                  variant="outline"
                  onClick={() => {
                    setOptimizerPresetRun(null);
                    setOptimizerOpen(true);
                  }}
                  disabled={!prompt}
                >
                  <WandSparkles className="h-4 w-4" />
                  {prompt?.source === 'official'
                    ? t('actions.optimizeAsPersonal')
                    : t('actions.optimizePrompt')}
                </Button>
                {canManagePromptDetails ? (
                  <Button variant="outline" onClick={() => setMetadataOpen(true)}>
                    <Settings2 className="h-4 w-4" />
                    {t('actions.editDetails')}
                  </Button>
                ) : null}
                {versions.length > 1 ? (
                  <Button variant="outline" onClick={() => setCompareOpen(true)} disabled={!prompt}>
                    <GitCompare className="h-4 w-4" />
                    {t('compare.trigger')}
                  </Button>
                ) : null}
                {canManage ? (
                  <>
                    {prompt?.source === 'personal' ? (
                      <Button
                        variant="outline"
                        onClick={() => setShareConfirmOpen(true)}
                        disabled={!prompt || updatePrompt.isPending}
                      >
                        <Share2 className="h-4 w-4" />
                        {t('actions.shareToWorkspace')}
                      </Button>
                    ) : null}
                    {prompt?.source !== 'official' ? (
                      <Button onClick={() => setDialogOpen(true)} disabled={!prompt}>
                        <PencilLine className="h-4 w-4" />
                        {t('actions.editContent')}
                      </Button>
                    ) : null}
                  </>
                ) : null}
              </div>
            </div>

            {prompt && currentVersion ? (
              <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_280px] gap-4 pt-1">
                <div className="rounded-lg border p-4 space-y-3">
                  <div className="flex items-center justify-between gap-3 flex-wrap">
                    <div className="space-y-1">
                      <div className="text-sm font-medium">{t('detail.currentVersion')}</div>
                      <div className="flex items-center gap-2 flex-wrap">
                        <Badge variant="secondary">v{currentVersion.version}</Badge>
                        <Badge variant="outline">
                          {t(promptTypeLabelKey(currentVersion.prompt_type))}
                        </Badge>
                      </div>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {new Date(currentVersion.updated_at).toLocaleString()}
                    </div>
                  </div>
                  <pre className="max-h-[420px] overflow-auto rounded-md border bg-muted/20 p-3 text-xs whitespace-pre-wrap break-words">
                    {stringifyPromptContent(currentVersion.content)}
                  </pre>
                  {prompt.source !== 'official' ? (
                    <div className="rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
                      {t('detail.latestVersionBehavior')}
                    </div>
                  ) : null}
                </div>

                <div className="rounded-lg border bg-muted/20 p-4 space-y-4">
                  <div>
                    <div className="text-sm font-medium">{t('detail.atAGlance')}</div>
                    <p className="text-xs text-muted-foreground mt-1">
                      {t('detail.atAGlanceDescription')}
                    </p>
                  </div>
                  <div className="grid grid-cols-2 gap-3 text-sm">
                    <div>
                      <div className="text-xs text-muted-foreground">{t('fields.locale')}</div>
                      <div className="font-medium">{t(promptLocaleLabelKey(prompt.locale))}</div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">{t('fields.source')}</div>
                      <div className="font-medium">{t(`sources.${prompt.source}`)}</div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">
                        {t('detail.versionCount')}
                      </div>
                      <div className="font-medium">{versions.length}</div>
                    </div>
                  </div>
                  <div className="space-y-3 border-t pt-3">
                    <div>
                      <div className="text-xs font-medium text-muted-foreground">
                        {t('detail.releaseStatus')}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('detail.releaseStatusDescription')}
                      </p>
                    </div>
                    {hasSingleVersion ? (
                      <div className="rounded-md border bg-background px-3 py-2 text-xs leading-5 text-muted-foreground">
                        <div className="font-medium text-foreground">
                          {t('library.singleVersionLabel')} v{currentVersion.version}
                        </div>
                        <div className="mt-1">{t('detail.singleVersionReleaseDescription')}</div>
                      </div>
                    ) : (
                      <div className="grid grid-cols-2 gap-3 text-sm">
                        <div>
                          <div className="text-xs text-muted-foreground">
                            {t('detail.latestTarget')}
                          </div>
                          <div className="font-medium">
                            {currentVersion ? `v${currentVersion.version}` : '-'}
                          </div>
                        </div>
                        <div>
                          <div className="text-xs text-muted-foreground">
                            {t('detail.onlineTarget')}
                          </div>
                          <div className="font-medium">
                            {onlineVersion ? `v${onlineVersion.version}` : t('detail.unsetTarget')}
                          </div>
                        </div>
                      </div>
                    )}
                    {!hasSingleVersion && releaseImpactDescription ? (
                      <div className="rounded-md border bg-background px-3 py-2 text-xs leading-5 text-muted-foreground">
                        {releaseImpactDescription}
                      </div>
                    ) : null}
                    {canManagePromptDetails && currentVersion ? (
                      <Button
                        data-testid="prompt-current-publish-online"
                        variant="outline"
                        size="sm"
                        className="w-full"
                        onClick={() => openPublishConfirm(currentVersion)}
                        disabled={
                          setPromptLabels.isPending ||
                          onlineVersion?.version === currentVersion.version
                        }
                      >
                        {onlineVersion?.version === currentVersion.version
                          ? t('detail.currentOnline')
                          : t('detail.makeCurrentOnline')}
                      </Button>
                    ) : null}
                  </div>
                  {prompt.source === 'official' ? (
                    <div className="rounded-lg border bg-background px-3 py-3 text-xs text-muted-foreground">
                      {t('detail.officialReadOnlyNote')}
                    </div>
                  ) : null}
                  {assetHealth ? (
                    <div className="space-y-3 border-t pt-3">
                      <div>
                        <div className="text-xs font-medium text-muted-foreground">
                          {t('detail.assetImpact')}
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {t('detail.assetImpactDescription')}
                        </p>
                      </div>
                      <div
                        className={`rounded-md border px-3 py-3 text-xs leading-5 ${
                          assetHealth.kind === 'risk'
                            ? 'border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/20 dark:text-amber-200'
                            : 'bg-background text-muted-foreground'
                        }`}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="flex gap-2">
                            {assetHealth.kind === 'risk' ? (
                              <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0" />
                            ) : assetHealth.kind === 'active' ? (
                              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" />
                            ) : (
                              <Link2 className="mt-0.5 h-4 w-4 shrink-0" />
                            )}
                            <div>
                              <div
                                className={`font-medium ${
                                  assetHealth.kind === 'risk' ? '' : 'text-foreground'
                                }`}
                              >
                                {assetHealth.title}
                              </div>
                              <div className="mt-1">{assetHealth.description}</div>
                            </div>
                          </div>
                          <Badge
                            variant={assetHealth.kind === 'active' ? 'secondary' : 'outline'}
                            className="shrink-0"
                          >
                            {assetHealth.badge}
                          </Badge>
                        </div>
                      </div>
                      <div className="grid grid-cols-2 gap-3 text-sm">
                        <div>
                          <div className="text-xs text-muted-foreground">
                            {t('usage.metrics.linkedNodes')}
                          </div>
                          <div className="font-medium">{usage?.linked_nodes_count ?? 0}</div>
                        </div>
                        <div>
                          <div className="text-xs text-muted-foreground">
                            {t('usage.metrics.totalRuns')}
                          </div>
                          <div className="font-medium">{usage?.total_run_count ?? 0}</div>
                        </div>
                      </div>
                      {visibleReferences.length > 0 ? (
                        <div className="space-y-2">
                          {visibleReferences.map(reference => {
                            const referenceTarget = formatReferenceTarget(reference);
                            const isLatestReference =
                              reference.reference_mode === 'label' &&
                              reference.label?.toLowerCase() === 'latest';
                            return (
                              <Link
                                key={`${reference.workflow_id}-${reference.node_id}`}
                                href={`/console/agents/${reference.agent_id}/workflow?nodeId=${reference.node_id}`}
                                className="block rounded-md border bg-background px-3 py-2 text-xs transition-colors hover:border-primary/40 hover:text-primary"
                              >
                                <span className="flex items-center justify-between gap-2">
                                  <span className="font-medium text-foreground">
                                    {reference.agent_name}
                                  </span>
                                  {isLatestReference ? (
                                    <Badge variant="warning" className="shrink-0">
                                      {t('detail.referenceLatestRisk')}
                                    </Badge>
                                  ) : null}
                                </span>
                                <span className="mt-1 block text-muted-foreground">
                                  {reference.node_title || reference.node_id}
                                </span>
                                {referenceTarget ? (
                                  <span className="mt-1 block text-muted-foreground">
                                    {t('detail.referenceTarget', { target: referenceTarget })}
                                  </span>
                                ) : null}
                              </Link>
                            );
                          })}
                          {hiddenReferenceCount > 0 ? (
                            <div className="text-xs text-muted-foreground">
                              {t('detail.moreReferences', { count: hiddenReferenceCount })}
                            </div>
                          ) : null}
                        </div>
                      ) : (
                        <div className="text-xs text-muted-foreground">
                          {t('usage.emptyReferences')}
                        </div>
                      )}
                      <div className="text-xs text-muted-foreground">
                        {usage?.last_run_at
                          ? t('detail.lastRunAt', {
                              time: new Date(usage.last_run_at).toLocaleString(),
                            })
                          : t('usage.emptyRuns')}
                      </div>
                    </div>
                  ) : null}
                  {relatedTemplates.length > 0 ? (
                    <div className="space-y-2 border-t pt-3">
                      <div className="flex items-center justify-between gap-3">
                        <div className="text-xs font-medium text-muted-foreground">
                          {t('relatedTemplates.title')}
                        </div>
                        <Link
                          href="/console/agents"
                          className="text-xs text-primary hover:underline"
                        >
                          {t('relatedTemplates.openInGallery')}
                        </Link>
                      </div>
                      <div className="space-y-1">
                        {relatedTemplates.slice(0, 2).map(template => {
                          const copy = getTemplateCopy(templateT, template);
                          return (
                            <Link
                              key={template.id}
                              href={`/console/agents?template=${template.id}`}
                              className="block rounded-md px-2 py-2 transition-colors hover:bg-background"
                            >
                              <div className="text-sm font-medium">{copy.title}</div>
                              <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                                {copy.description}
                              </div>
                            </Link>
                          );
                        })}
                      </div>
                    </div>
                  ) : null}
                </div>
              </div>
            ) : null}
          </div>

          {isLoading || !prompt ? (
            <div className="text-sm text-muted-foreground">{t('states.loading')}</div>
          ) : (
            <div className="space-y-5">
              {previousVersions.length > 0 ? (
                <div className="rounded-xl border p-4 space-y-4">
                  <div className="flex items-start justify-between gap-3 flex-wrap">
                    <div>
                      <h2 className="text-lg font-semibold">{t('detail.previousVersions')}</h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('detail.previousVersionsDescription')}
                      </p>
                    </div>
                    <Badge variant="outline">
                      {t('detail.previousVersionsCount', { count: previousVersions.length })}
                    </Badge>
                  </div>
                  <div className="grid grid-cols-1 lg:grid-cols-[320px_minmax(0,1fr)] gap-4">
                    <div className="space-y-2">
                      {previousVersions.map(version => {
                        const isSelected = selectedPreviousVersion?.version === version.version;
                        return (
                          <button
                            key={version.id}
                            type="button"
                            aria-pressed={isSelected}
                            aria-label={t('detail.selectVersionPreview', {
                              version: `v${version.version}`,
                            })}
                            onClick={() => setSelectedHistoryVersion(version.version)}
                            className={`w-full rounded-lg border px-3 py-3 text-left transition-colors ${
                              isSelected
                                ? 'border-primary/50 bg-primary/5'
                                : 'bg-background hover:border-primary/30'
                            }`}
                          >
                            <div className="flex items-start justify-between gap-3">
                              <div className="space-y-1 min-w-0">
                                <div className="font-medium">v{version.version}</div>
                                {version.commit_message ? (
                                  <div className="line-clamp-2 text-xs text-muted-foreground">
                                    {version.commit_message}
                                  </div>
                                ) : null}
                              </div>
                              <Badge variant="secondary">
                                {t(promptTypeLabelKey(version.prompt_type))}
                              </Badge>
                            </div>
                            <div className="mt-3 flex items-center gap-2 flex-wrap">
                              {version.labels.length > 0 ? (
                                version.labels.map(label => (
                                  <Badge key={`${version.id}-${label}`} variant="outline">
                                    {displayVersionLabel(label)}
                                  </Badge>
                                ))
                              ) : (
                                <Badge variant="outline">{t('detail.noVersionLabels')}</Badge>
                              )}
                            </div>
                            <div className="mt-2 text-xs text-muted-foreground">
                              {new Date(version.updated_at).toLocaleString()}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                    {selectedPreviousVersion ? (
                      <div className="rounded-lg border bg-muted/20 p-4 space-y-3 min-w-0">
                        <div className="flex items-start justify-between gap-3 flex-wrap">
                          <div className="space-y-1">
                            <div className="text-sm font-medium">
                              {t('detail.versionPreview', {
                                version: `v${selectedPreviousVersion.version}`,
                              })}
                            </div>
                            <div className="flex items-center gap-2 flex-wrap">
                              <Badge variant="secondary">
                                {t(promptTypeLabelKey(selectedPreviousVersion.prompt_type))}
                              </Badge>
                              {selectedPreviousVersion.labels.map(label => (
                                <Badge
                                  key={`${selectedPreviousVersion.id}-${label}`}
                                  variant="outline"
                                >
                                  {displayVersionLabel(label)}
                                </Badge>
                              ))}
                            </div>
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {new Date(selectedPreviousVersion.updated_at).toLocaleString()}
                          </div>
                        </div>
                        {canManagePromptDetails ? (
                          <Button
                            data-testid="prompt-history-publish-online"
                            variant="outline"
                            size="sm"
                            className="w-full"
                            onClick={() => openPublishConfirm(selectedPreviousVersion)}
                            disabled={
                              setPromptLabels.isPending ||
                              onlineVersion?.version === selectedPreviousVersion.version
                            }
                          >
                            {onlineVersion?.version === selectedPreviousVersion.version
                              ? t('detail.versionOnline')
                              : t('detail.makeVersionOnline')}
                          </Button>
                        ) : null}
                        {selectedPreviousVersion.commit_message ? (
                          <div className="rounded-md border bg-background px-3 py-2 text-xs text-muted-foreground">
                            {selectedPreviousVersion.commit_message}
                          </div>
                        ) : null}
                        <pre className="max-h-[360px] overflow-auto rounded-md border bg-background p-3 text-xs whitespace-pre-wrap break-words">
                          {stringifyPromptContent(selectedPreviousVersion.content)}
                        </pre>
                      </div>
                    ) : null}
                  </div>
                </div>
              ) : null}

              <PromptOptimizationHistory
                promptId={promptId}
                promptSource={prompt.source}
                promptVersions={versions}
                canManage={canManage}
                hideWhenEmpty
                onRetryRun={run => {
                  setOptimizerPresetRun(run);
                  setOptimizerOpen(true);
                }}
              />
            </div>
          )}
        </div>

        {prompt ? (
          <>
            <PromptVersionDialog
              open={dialogOpen}
              onOpenChange={setDialogOpen}
              defaultType={currentVersion?.prompt_type ?? prompt.latest_prompt_type}
              initialContent={currentVersion?.content}
              initialCommitMessage={t('detail.editVersionCommitMessage')}
              title={t('versions.editTitle')}
              description={t('versions.editDescription')}
              submitLabel={t('versions.saveAsVersion')}
              governanceNote={
                <div className="space-y-3">
                  <div className="font-medium text-foreground">{t('versions.governanceTitle')}</div>
                  <div className="grid grid-cols-3 gap-3">
                    <div>
                      <div className="text-xs">{t('versions.currentLatest')}</div>
                      <div className="mt-1 font-medium text-foreground">
                        {currentVersion ? `v${currentVersion.version}` : '-'}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs">{t('versions.currentOnline')}</div>
                      <div className="mt-1 font-medium text-foreground">
                        {onlineVersion ? `v${onlineVersion.version}` : t('detail.unsetTarget')}
                      </div>
                    </div>
                    <div>
                      <div className="text-xs">{t('versions.afterSave')}</div>
                      <div className="mt-1 font-medium text-foreground">v{nextVersionNumber}</div>
                    </div>
                  </div>
                  <p className="text-xs leading-5">{t('versions.governanceNote')}</p>
                </div>
              }
              onSubmit={async payload => {
                await createVersion.mutateAsync(payload);
              }}
            />
            <PromptOptimizerDialog
              open={optimizerOpen}
              onOpenChange={setOptimizerOpen}
              initialPrompt={optimizerPresetRun?.raw_prompt ?? latestVersionText}
              promptId={promptId}
              sourceHelpText={
                prompt.source === 'official' ? t('optimizer.officialTemplateHelp') : undefined
              }
              onApplyResult={handleApplyOptimizedPrompt}
              applyLabel={
                prompt.source === 'official'
                  ? t('optimizer.saveAsPersonalPrompt')
                  : t('optimizer.saveAsNewVersion')
              }
              initialGoal={optimizerPresetRun?.goal}
              initialPreserveVariables={optimizerPresetRun?.preserve_variables}
              initialModel={
                optimizerPresetRun?.provider && optimizerPresetRun?.model
                  ? {
                      provider: optimizerPresetRun.provider,
                      model: optimizerPresetRun.model,
                    }
                  : null
              }
            />
            <PromptVersionCompareDialog
              open={compareOpen}
              onOpenChange={setCompareOpen}
              versions={versions}
            />
            <Dialog open={metadataOpen} onOpenChange={setMetadataOpen}>
              <DialogContent size="lg">
                <DialogHeader>
                  <DialogTitle>{t('detail.metadata')}</DialogTitle>
                  <DialogDescription>{t('detail.metadataDescription')}</DialogDescription>
                </DialogHeader>
                <DialogBody className="space-y-4">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <Label>{t('fields.name')}</Label>
                      <Input value={nameDraft} onChange={e => setNameDraft(e.target.value)} />
                    </div>
                    <div className="space-y-2">
                      <Label>{t('fields.locale')}</Label>
                      <Select value={localeDraft} onValueChange={setLocaleDraft}>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="zh-Hans">{t('localeOptions.zhHans')}</SelectItem>
                          <SelectItem value="en-US">{t('localeOptions.enUS')}</SelectItem>
                          <SelectItem value="ja-JP">{t('localeOptions.jaJP')}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <Label>{t('fields.category')}</Label>
                      <Input
                        value={categoryDraft}
                        onChange={e => setCategoryDraft(e.target.value)}
                      />
                    </div>
                    <div className="space-y-2">
                      <Label>{t('fields.source')}</Label>
                      <Select
                        value={sourceDraft}
                        onValueChange={value => setSourceDraft(value as 'personal' | 'workspace')}
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="personal">{t('sources.personal')}</SelectItem>
                          <SelectItem value="workspace">{t('sources.workspace')}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.tags')}</Label>
                    <Input value={tagsDraft} onChange={e => setTagsDraft(e.target.value)} />
                  </div>
                  <div className="space-y-2">
                    <Label>{t('fields.description')}</Label>
                    <Textarea
                      value={descriptionDraft}
                      onChange={e => setDescriptionDraft(e.target.value)}
                      className="min-h-28"
                    />
                  </div>
                </DialogBody>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setMetadataOpen(false)}>
                    {t('actions.cancel')}
                  </Button>
                  <Button
                    onClick={async () => {
                      try {
                        await updatePrompt.mutateAsync({
                          data: {
                            name: nameDraft.trim(),
                            description: descriptionDraft.trim() || null,
                            locale: localeDraft,
                            category: categoryDraft.trim() || null,
                            tags: tagsDraft
                              .split(',')
                              .map(item => item.trim())
                              .filter(Boolean),
                            source: sourceDraft,
                          },
                        });
                        setMetadataOpen(false);
                      } catch {
                        // The mutation hook already shows a localized error toast.
                      }
                    }}
                    disabled={updatePrompt.isPending || !nameDraft.trim()}
                  >
                    <Save className="h-4 w-4" />
                    {t('actions.saveMeta')}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
            <ConfirmDialog
              open={publishConfirmOpen}
              onOpenChange={open => {
                setPublishConfirmOpen(open);
                if (!open) setPublishTargetVersion(null);
              }}
              title={t('detail.publishOnlineConfirmTitle')}
              description={
                <>
                  {t('detail.publishOnlineConfirmDescription', {
                    version: publishTargetVersion ? `v${publishTargetVersion.version}` : '',
                  })}
                  <br />
                  {publishImpactDescription}
                </>
              }
              confirmText={t('detail.publishOnlineConfirmAction', {
                version: publishTargetVersion ? `v${publishTargetVersion.version}` : '',
              })}
              cancelText={t('actions.cancel')}
              loading={setPromptLabels.isPending}
              onConfirm={handleSetVersionOnline}
            />
            <ConfirmDialog
              open={shareConfirmOpen}
              onOpenChange={setShareConfirmOpen}
              title={t('shareToWorkspaceConfirm.title')}
              description={t('shareToWorkspaceConfirm.description')}
              confirmText={t('actions.shareToWorkspace')}
              cancelText={t('actions.cancel')}
              loading={updatePrompt.isPending}
              onConfirm={handleShareToWorkspace}
            />
          </>
        ) : null}
      </>
    </WorkspaceMismatchGuard>
  );
}
