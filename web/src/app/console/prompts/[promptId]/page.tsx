'use client';

import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'next/navigation';
import { ArrowLeft, Copy, FlaskConical, GitCompare, Plus, Save, ShieldAlert, Share2, WandSparkles } from 'lucide-react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
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
  useCreatePromptVersion,
  useSetPromptLabels,
  useUpdatePrompt,
} from '@/hooks/prompt/use-prompts';
import { PromptVersionDialog } from '@/components/prompts/prompt-version-dialog';
import { PromptOptimizationHistory } from '@/components/prompts/prompt-optimization-history';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { PromptReleaseChannels } from '@/components/prompts/prompt-release-channels';
import { PromptUsageSummary } from '@/components/prompts/prompt-usage-summary';
import { PromptVersionCompareDialog } from '@/components/prompts/prompt-version-compare-dialog';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { toast } from 'sonner';
import { findTemplatesByPromptId } from '@/components/agents/templates/template-manifest';
import { getTemplateCopy, type TemplateTranslator } from '@/components/agents/templates/template-labels';
import type { PromptOptimizationRun } from '@/services/types/prompt';

export default function PromptDetailPage() {
  const t = useT('prompts');
  const rootT = useT();
  const params = useParams<{ promptId: string }>();
  const promptId = params?.promptId ?? '';
  const templateT = rootT as unknown as TemplateTranslator;
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('agent.view');
  const canManage = hasPermission('agent.manage');
  const { prompt, isLoading } = usePrompt(promptId, canView);
  const createVersion = useCreatePromptVersion(promptId);
  const setLabels = useSetPromptLabels(promptId);
  const updatePrompt = useUpdatePrompt(promptId);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [optimizerOpen, setOptimizerOpen] = useState(false);
  const [compareOpen, setCompareOpen] = useState(false);
  const [labelDrafts, setLabelDrafts] = useState<Record<number, string>>({});
  const [nameDraft, setNameDraft] = useState('');
  const [descriptionDraft, setDescriptionDraft] = useState('');
  const [localeDraft, setLocaleDraft] = useState('zh-Hans');
  const [categoryDraft, setCategoryDraft] = useState('');
  const [tagsDraft, setTagsDraft] = useState('');
  const [sourceDraft, setSourceDraft] = useState<'personal' | 'workspace' | 'official'>('personal');
  const [optimizerPresetRun, setOptimizerPresetRun] = useState<PromptOptimizationRun | null>(null);

  const versions = useMemo(() => prompt?.versions ?? [], [prompt?.versions]);
  const relatedTemplates = useMemo(() => findTemplatesByPromptId(promptId), [promptId]);
  const latestVersionText = useMemo(() => {
    const latest = versions[0];
    if (!latest) return '';
    return typeof latest.content === 'string'
      ? latest.content
      : JSON.stringify(latest.content, null, 2);
  }, [versions]);

  useEffect(() => {
    if (!prompt) return;
    setNameDraft(prompt.name);
    setDescriptionDraft(prompt.description ?? '');
    setLocaleDraft(prompt.locale);
    setCategoryDraft(prompt.category ?? '');
    setTagsDraft(prompt.tags.join(', '));
    setSourceDraft(prompt.source);
  }, [prompt]);

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
    <>
      <div className="p-4 sm:p-6 lg:p-8 space-y-6 flex flex-col h-full overflow-y-auto">
        <div className="flex items-center justify-between gap-4">
          <div className="space-y-2">
            <Link
              href="/console/prompts"
              className="text-sm text-muted-foreground inline-flex items-center gap-1 hover:text-foreground"
            >
              <ArrowLeft className="h-4 w-4" />
              {t('actions.back')}
            </Link>
            <div className="flex items-center gap-2 flex-wrap">
              <h1 className="text-2xl font-semibold">{prompt?.name ?? t('states.loading')}</h1>
              {prompt ? <Badge variant="secondary">{t(`sources.${prompt.source}`)}</Badge> : null}
              {prompt ? <Badge variant="outline">{prompt.locale}</Badge> : null}
            </div>
            {prompt?.description ? <p className="text-sm text-muted-foreground">{prompt.description}</p> : null}
          </div>
          <div className="flex items-center gap-2">
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
            <Button
              variant="outline"
              onClick={() => {
                setOptimizerPresetRun(null);
                setOptimizerOpen(true);
              }}
              disabled={!prompt}
            >
              <WandSparkles className="h-4 w-4" />
              {t('actions.optimizePrompt')}
            </Button>
            <Button asChild variant="outline" disabled={!prompt}>
              <Link href={`/console/prompts?tab=playground&promptId=${promptId}`}>
                <FlaskConical className="h-4 w-4" />
                {t('actions.testInPlayground')}
              </Link>
            </Button>
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
                    onClick={async () => {
                      try {
                        await updatePrompt.mutateAsync({ source: 'workspace' });
                        toast.success(t('messages.shareToWorkspaceSuccess'));
                      } catch {
                        toast.error(t('messages.shareToWorkspaceFailed'));
                      }
                    }}
                    disabled={!prompt || updatePrompt.isPending}
                  >
                    <Share2 className="h-4 w-4" />
                    {t('actions.shareToWorkspace')}
                  </Button>
                ) : null}
                {prompt?.source !== 'official' ? (
                  <Button onClick={() => setDialogOpen(true)} disabled={!prompt}>
                    <Plus className="h-4 w-4" />
                    {t('actions.newVersion')}
                  </Button>
                ) : null}
              </>
            ) : null}
          </div>
        </div>

        {isLoading || !prompt ? (
          <div className="text-sm text-muted-foreground">{t('states.loading')}</div>
        ) : (
          <div className="space-y-6">
            {relatedTemplates.length > 0 ? (
              <div className="rounded-xl border p-4 space-y-3">
                <div className="flex items-center justify-between gap-3">
                  <h2 className="text-lg font-semibold">{t('relatedTemplates.title')}</h2>
                  <Link href="/console/agents" className="text-sm text-primary hover:underline">
                    {t('relatedTemplates.openInGallery')}
                  </Link>
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {relatedTemplates.map(template => {
                    const copy = getTemplateCopy(templateT, template);
                    return (
                      <Link
                        key={template.id}
                        href={`/console/agents?template=${template.id}`}
                        className="rounded-lg border p-3 hover:border-primary/40 hover:bg-muted/20 transition-colors"
                      >
                        <div className="font-medium">{copy.title}</div>
                        <div className="text-sm text-muted-foreground mt-1 line-clamp-2">
                          {copy.description}
                        </div>
                      </Link>
                    );
                  })}
                </div>
              </div>
            ) : null}
            <PromptUsageSummary promptId={promptId} enabled={canView} versions={versions} />
            <PromptReleaseChannels
              versions={versions}
              promptSource={prompt.source}
              canManage={canManage}
              isPending={setLabels.isPending}
              onSaveLabels={async (version, labels) => {
                await setLabels.mutateAsync({ version, labels });
              }}
            />
            <div className="rounded-xl border p-4 space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>{t('fields.name')}</Label>
                  <Input
                    value={nameDraft}
                    onChange={e => setNameDraft(e.target.value)}
                    disabled={!canManage || prompt.source === 'official'}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('fields.locale')}</Label>
                  <Select
                    value={localeDraft}
                    onValueChange={setLocaleDraft}
                    disabled={!canManage || prompt.source === 'official'}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="zh-Hans">zh-Hans</SelectItem>
                      <SelectItem value="en-US">en-US</SelectItem>
                      <SelectItem value="ja-JP">ja-JP</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label>{t('fields.category')}</Label>
                  <Input
                    value={categoryDraft}
                    onChange={e => setCategoryDraft(e.target.value)}
                    disabled={!canManage || prompt.source === 'official'}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t('fields.source')}</Label>
                  <Select
                    value={sourceDraft}
                    onValueChange={value => setSourceDraft(value as 'personal' | 'workspace' | 'official')}
                    disabled={!canManage || prompt.source === 'official'}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="official">{t('sources.official')}</SelectItem>
                      <SelectItem value="personal">{t('sources.personal')}</SelectItem>
                      <SelectItem value="workspace">{t('sources.workspace')}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="space-y-2">
                <Label>{t('fields.tags')}</Label>
                <Input
                  value={tagsDraft}
                  onChange={e => setTagsDraft(e.target.value)}
                  disabled={!canManage || prompt.source === 'official'}
                />
              </div>
              <div className="space-y-2">
                <Label>{t('fields.description')}</Label>
                <Textarea
                  value={descriptionDraft}
                  onChange={e => setDescriptionDraft(e.target.value)}
                  disabled={!canManage || prompt.source === 'official'}
                  className="min-h-24"
                />
              </div>
              {canManage && prompt.source !== 'official' ? (
                <div className="flex justify-end">
                  <Button
                    onClick={async () => {
                      await updatePrompt.mutateAsync({
                        name: nameDraft.trim(),
                        description: descriptionDraft.trim() || null,
                        locale: localeDraft,
                        category: categoryDraft.trim() || null,
                        tags: tagsDraft
                          .split(',')
                          .map(item => item.trim())
                          .filter(Boolean),
                        ...(sourceDraft === 'official' ? {} : { source: sourceDraft }),
                      });
                    }}
                    disabled={updatePrompt.isPending || !nameDraft.trim()}
                  >
                    <Save className="h-4 w-4" />
                    {t('actions.saveMeta')}
                  </Button>
                </div>
              ) : null}
            </div>

            <PromptOptimizationHistory
              promptId={promptId}
              promptSource={prompt.source}
              promptVersions={versions}
              canManage={canManage}
              onRetryRun={run => {
                setOptimizerPresetRun(run);
                setOptimizerOpen(true);
              }}
            />

            <div className="space-y-4">
            {versions.map(version => (
              <div key={version.id} className="rounded-xl border p-4 space-y-4">
                <div className="flex items-center justify-between gap-3 flex-wrap">
                  <div className="flex items-center gap-2 flex-wrap">
                    <div className="font-medium">v{version.version}</div>
                    <Badge variant="secondary">{version.prompt_type}</Badge>
                    {version.labels.map(label => (
                      <Badge key={label} variant={label === 'production' ? 'default' : 'outline'}>
                        {label}
                      </Badge>
                    ))}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {new Date(version.updated_at).toLocaleString()}
                  </div>
                </div>
                <pre className="rounded-md border bg-muted/20 p-3 text-xs whitespace-pre-wrap break-words">
                  {typeof version.content === 'string'
                    ? version.content
                    : JSON.stringify(version.content, null, 2)}
                </pre>
                {canManage ? (
                  <div className="flex flex-col md:flex-row gap-2">
                    <Input
                      value={labelDrafts[version.version] ?? version.labels.join(', ')}
                      onChange={e =>
                        setLabelDrafts(prev => ({
                          ...prev,
                          [version.version]: e.target.value,
                        }))
                      }
                      placeholder={t('placeholders.labels')}
                    />
                    <Button
                      variant="outline"
                      onClick={() =>
                        setLabels.mutate({
                          version: version.version,
                          labels: (labelDrafts[version.version] ?? version.labels.join(', '))
                            .split(',')
                            .map(item => item.trim())
                            .filter(Boolean),
                        })
                      }
                    >
                      <Save className="h-4 w-4" />
                      {t('actions.saveLabels')}
                    </Button>
                  </div>
                ) : null}
              </div>
            ))}
            </div>
          </div>
        )}
      </div>

      {prompt ? (
        <>
          <PromptVersionDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            defaultType={prompt.latest_prompt_type}
            onSubmit={async payload => {
              await createVersion.mutateAsync(payload);
            }}
          />
          <PromptOptimizerDialog
            open={optimizerOpen}
            onOpenChange={setOptimizerOpen}
            initialPrompt={optimizerPresetRun?.raw_prompt ?? latestVersionText}
            promptId={promptId}
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
          <PromptVersionCompareDialog open={compareOpen} onOpenChange={setCompareOpen} versions={versions} />
        </>
      ) : null}
    </>
  );
}
