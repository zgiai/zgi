'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { FlaskConical, Plus, RefreshCw, ShieldAlert, WandSparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { usePrompts, useCreatePrompt } from '@/hooks/prompt/use-prompts';
import { PromptFormDialog } from '@/components/prompts/prompt-form-dialog';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { PromptPlaygroundPanel } from '@/components/prompts/prompt-playground-panel';
import { PromptPickerDialog } from '@/components/prompts/prompt-picker-dialog';
import {
  promptLocaleLabelKey,
  promptTypeLabelKey,
} from '@/components/prompts/prompt-display-labels';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import type { PromptPickerSelection } from '@/services/types/prompt';

function matchesLocale(currentLocale: string, promptLocale: string): boolean {
  const normalizedCurrent = currentLocale.trim().toLowerCase();
  const normalizedPrompt = promptLocale.trim().toLowerCase();
  if (!normalizedCurrent || !normalizedPrompt) {
    return false;
  }
  if (normalizedCurrent === normalizedPrompt) {
    return true;
  }

  const currentBase = normalizedCurrent.split('-')[0];
  const promptBase = normalizedPrompt.split('-')[0];
  return currentBase === promptBase;
}

export default function PromptsPage() {
  const t = useT('prompts');
  const { locale } = useLocale();
  const searchParams = useSearchParams();
  const currentWorkspace = useCurrentWorkspace();
  const { hasWorkspaceAccess, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canUseWorkspaceTools = Boolean(currentWorkspace?.id) && hasWorkspaceAccess();
  const canView = canUseWorkspaceTools;
  const canManage = canUseWorkspaceTools;
  const [keyword, setKeyword] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [optimizerOpen, setOptimizerOpen] = useState(false);
  const [promptPickerOpen, setPromptPickerOpen] = useState(false);
  const [playgroundSelection, setPlaygroundSelection] = useState<PromptPickerSelection | null>(
    null
  );
  const [activeTab, setActiveTab] = useState<'library' | 'playground'>(
    searchParams.get('tab') === 'playground' ? 'playground' : 'library'
  );
  const prefillPromptId = searchParams.get('promptId') ?? '';

  useEffect(() => {
    if (searchParams.get('tab') === 'playground') {
      setActiveTab('playground');
      return;
    }
    setActiveTab('library');
  }, [searchParams]);

  useEffect(() => {
    setPlaygroundSelection(null);
  }, [prefillPromptId]);

  const { prompts, isLoading, isFetching, error, refetch } = usePrompts(
    {
      keyword: keyword || undefined,
      workspace_id: currentWorkspace?.id,
      limit: 50,
    },
    canView
  );
  const createPrompt = useCreatePrompt();

  const filteredOfficialPrompts = useMemo(() => {
    const officialPrompts = prompts.filter(prompt => prompt.source === 'official');
    const matched = officialPrompts.filter(prompt => matchesLocale(locale, prompt.locale));
    return matched.length > 0 ? matched : officialPrompts;
  }, [locale, prompts]);

  const grouped = useMemo(
    () => ({
      official: filteredOfficialPrompts,
      workspace: prompts.filter(prompt => prompt.source === 'workspace'),
      personal: prompts.filter(prompt => prompt.source === 'personal'),
    }),
    [filteredOfficialPrompts, prompts]
  );

  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('states.accessDeniedTitle')}</h2>
        <p className="text-muted-foreground max-w-md">{t('states.accessDeniedDescription')}</p>
      </div>
    );
  }

  const hasSearchKeyword = keyword.trim().length > 0;
  const empty = prompts.length === 0 && !isLoading && !error;
  const playgroundSelectionLabel = playgroundSelection
    ? [
        playgroundSelection.prompt.name,
        t('library.currentVersion', {
          version: playgroundSelection.version.version,
        }),
      ].join(' · ')
    : undefined;
  const playgroundSelectionText =
    playgroundSelection && typeof playgroundSelection.version.content === 'string'
      ? playgroundSelection.version.content
      : undefined;
  const playgroundSelectionMessages =
    playgroundSelection && typeof playgroundSelection.version.content !== 'string'
      ? playgroundSelection.version.content
      : undefined;

  return (
    <>
      <div className="flex h-full flex-col space-y-6 overflow-y-auto bg-background p-4 sm:p-6 lg:p-8">
        <Tabs
          value={activeTab}
          onValueChange={value => setActiveTab(value as 'library' | 'playground')}
        >
          <div className="flex flex-col xl:flex-row xl:items-center justify-between gap-4">
            <div className="flex flex-col gap-3">
              <div>
                <h1 className="text-2xl font-semibold">{t('title')}</h1>
                <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{t('description')}</p>
              </div>
              <div className="flex flex-wrap items-center gap-3">
                <TabsList>
                  <TabsTrigger value="library">{t('tabs.library')}</TabsTrigger>
                  <TabsTrigger value="playground">{t('tabs.playground')}</TabsTrigger>
                </TabsList>
                {activeTab === 'library' ? (
                  <Button
                    isIcon
                    variant="ghost"
                    className="size-7 rounded-sm hover:bg-muted"
                    onClick={() => void refetch()}
                    disabled={isFetching}
                  >
                    <RefreshCw size={16} className={isFetching ? 'animate-spin' : ''} />
                  </Button>
                ) : null}
              </div>
            </div>
            {activeTab === 'library' ? (
              <div className="flex w-full flex-col gap-3 sm:flex-row sm:flex-wrap xl:w-auto xl:justify-end">
                <Input
                  value={keyword}
                  onChange={e => setKeyword(e.target.value)}
                  placeholder={t('search.placeholder')}
                  className="w-full sm:min-w-64 sm:flex-1 xl:w-72 xl:flex-none"
                />
                <Button
                  variant="outline"
                  className="shrink-0"
                  onClick={() => setOptimizerOpen(true)}
                >
                  <WandSparkles className="h-4 w-4" />
                  {t('actions.optimizePrompt')}
                </Button>
                {canManage ? (
                  <Button className="shrink-0" onClick={() => setDialogOpen(true)}>
                    <Plus className="h-4 w-4" />
                    {t('actions.newPrompt')}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>

          <TabsContent value="library" className="space-y-6">
            <section className="space-y-1">
              <h2 className="text-lg font-semibold">{t('tabs.library')}</h2>
              <p className="text-sm text-muted-foreground">{t('tabs.libraryDescription')}</p>
            </section>
            {error ? (
              <Alert variant="destructive">
                <AlertTitle>{t('states.loadFailedTitle')}</AlertTitle>
                <AlertDescription className="space-y-3">
                  <p>{t('states.loadFailedDescription')}</p>
                  <p className="text-xs">{error}</p>
                  <Button
                    variant="outline"
                    size="sm"
                    className="w-fit"
                    onClick={() => void refetch()}
                    disabled={isFetching}
                  >
                    <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
                    {t('actions.retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : empty ? (
              <div className="rounded-xl border border-dashed p-8 text-center">
                <div className="mx-auto max-w-xl space-y-4">
                  <div className="space-y-2">
                    <h3 className="text-base font-semibold text-foreground">
                      {hasSearchKeyword ? t('states.emptySearchTitle') : t('states.emptyTitle')}
                    </h3>
                    <p className="text-sm text-muted-foreground">
                      {hasSearchKeyword
                        ? t('states.emptySearchDescription')
                        : t('states.emptyDescription')}
                    </p>
                  </div>
                  <div className="flex flex-col items-center justify-center gap-2 sm:flex-row">
                    {hasSearchKeyword ? (
                      <Button variant="outline" onClick={() => setKeyword('')}>
                        {t('actions.clearSearch')}
                      </Button>
                    ) : null}
                    <Button variant="outline" onClick={() => setOptimizerOpen(true)}>
                      <WandSparkles className="h-4 w-4" />
                      {t('actions.optimizePrompt')}
                    </Button>
                    <Button variant="outline" onClick={() => setActiveTab('playground')}>
                      <FlaskConical className="h-4 w-4" />
                      {t('actions.testInPlayground')}
                    </Button>
                    {canManage ? (
                      <Button onClick={() => setDialogOpen(true)}>
                        <Plus className="h-4 w-4" />
                        {t('actions.newPrompt')}
                      </Button>
                    ) : null}
                  </div>
                </div>
              </div>
            ) : (
              <div className="space-y-8">
                {(['official', 'workspace', 'personal'] as const).map(source =>
                  grouped[source].length > 0 ? (
                    <section key={source} className="space-y-3">
                      <div className="flex items-center gap-2">
                        <h2 className="text-lg font-semibold">{t(`sources.${source}`)}</h2>
                        <Badge variant="outline">{grouped[source].length}</Badge>
                      </div>
                      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
                        {grouped[source].map(prompt => {
                          const hasSingleVersion = prompt.latest_version <= 1;

                          return (
                            <div
                              key={prompt.id}
                              className="rounded-xl border bg-background p-4 transition-colors hover:border-primary/40 hover:bg-muted/20"
                            >
                              <div className="flex items-center gap-2 flex-wrap">
                                <Link
                                  href={`/console/prompts/${prompt.id}`}
                                  className="font-medium hover:underline"
                                >
                                  {prompt.name}
                                </Link>
                                <Badge variant="outline">
                                  {t(promptLocaleLabelKey(prompt.locale))}
                                </Badge>
                                <Badge variant="secondary">
                                  {t(promptTypeLabelKey(prompt.latest_prompt_type))}
                                </Badge>
                              </div>
                              <div className="text-sm text-muted-foreground mt-2 line-clamp-2">
                                {prompt.description || t('states.noDescription')}
                              </div>
                              {hasSingleVersion ? (
                                <div className="mt-3 rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
                                  <div>{t('library.singleVersionLabel')}</div>
                                  <div className="mt-1 font-medium text-foreground">
                                    v{prompt.latest_version}
                                  </div>
                                  <div className="mt-1 leading-5">
                                    {t('library.singleVersionDescription')}
                                  </div>
                                </div>
                              ) : (
                                <div className="mt-3 grid grid-cols-1 gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                                  <div className="rounded-md border bg-muted/20 px-3 py-2">
                                    <div>{t('library.latestVersionLabel')}</div>
                                    <div className="mt-1 font-medium text-foreground">
                                      v{prompt.latest_version}
                                    </div>
                                  </div>
                                  <div className="rounded-md border bg-muted/20 px-3 py-2">
                                    <div>{t('library.onlineVersionLabel')}</div>
                                    <div className="mt-1 font-medium text-foreground">
                                      {prompt.production_version
                                        ? `v${prompt.production_version}`
                                        : t('library.onlineVersionUnset')}
                                    </div>
                                  </div>
                                </div>
                              )}
                              <div className="flex items-center gap-2 flex-wrap mt-3">
                                {prompt.production_version ? (
                                  !hasSingleVersion ? (
                                    <Badge
                                      variant={
                                        prompt.production_version === prompt.latest_version
                                          ? 'secondary'
                                          : 'outline'
                                      }
                                    >
                                      {prompt.production_version === prompt.latest_version
                                        ? t('library.onlineSameAsLatest')
                                        : t('library.latestNotOnline')}
                                    </Badge>
                                  ) : null
                                ) : (
                                  <Badge variant="outline">{t('library.notPublished')}</Badge>
                                )}
                                <Badge variant="outline">
                                  {prompt.source === 'official'
                                    ? t('library.officialHint')
                                    : t('library.reusableHint')}
                                </Badge>
                              </div>
                              <div className="mt-4 flex flex-wrap items-center gap-2">
                                <Button asChild size="sm" variant="outline">
                                  <Link href={`/console/prompts/${prompt.id}`}>
                                    {t('actions.openPrompt')}
                                  </Link>
                                </Button>
                                <Button asChild size="sm" variant="ghost">
                                  <Link
                                    href={`/console/prompts?tab=playground&promptId=${prompt.id}`}
                                  >
                                    <FlaskConical className="h-4 w-4" />
                                    {t('actions.testInPlayground')}
                                  </Link>
                                </Button>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </section>
                  ) : null
                )}
              </div>
            )}
          </TabsContent>

          <TabsContent value="playground" className="space-y-6">
            <PromptPlaygroundPanel
              prefillPromptId={playgroundSelection?.prompt.id || prefillPromptId || undefined}
              prefillPromptText={playgroundSelectionText}
              prefillPromptMessages={playgroundSelectionMessages}
              prefillPromptLabel={playgroundSelectionLabel}
              onChoosePrompt={() => setPromptPickerOpen(true)}
            />
          </TabsContent>
        </Tabs>
      </div>

      <PromptFormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={async payload => {
          await createPrompt.mutateAsync({ data: payload });
        }}
      />

      <PromptOptimizerDialog open={optimizerOpen} onOpenChange={setOptimizerOpen} />

      <PromptPickerDialog
        open={promptPickerOpen}
        onOpenChange={setPromptPickerOpen}
        applyLabel={t('playground.useSelectedPrompt')}
        onApply={selection => {
          setPlaygroundSelection(selection);
          setActiveTab('playground');
        }}
      />
    </>
  );
}
