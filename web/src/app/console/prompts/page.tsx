'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Plus, RefreshCw, LibraryBig, ShieldAlert, WandSparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { usePrompts, useCreatePrompt } from '@/hooks/prompt/use-prompts';
import { PromptFormDialog } from '@/components/prompts/prompt-form-dialog';
import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { PromptPlaygroundPanel } from '@/components/prompts/prompt-playground-panel';
import { useCurrentWorkspace, useWorkspaceStore } from '@/store/workspace-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

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
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('agent.view');
  const canManage = hasPermission('agent.manage');
  const [keyword, setKeyword] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [optimizerOpen, setOptimizerOpen] = useState(false);
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

  const { prompts, isLoading, isFetching, refetch } = usePrompts(
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

  const empty = prompts.length === 0 && !isLoading;

  return (
    <>
      <div className="p-4 sm:p-6 lg:p-8 space-y-6 flex flex-col h-full overflow-y-auto">
        <Tabs value={activeTab} onValueChange={value => setActiveTab(value as 'library' | 'playground')}>
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-semibold">{t('title')}</h1>
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
            {activeTab === 'library' ? (
              <div className="flex flex-col sm:flex-row gap-3">
                <Input
                  value={keyword}
                  onChange={e => setKeyword(e.target.value)}
                  placeholder={t('search.placeholder')}
                  className="w-full sm:w-72"
                />
                <Button variant="outline" onClick={() => setOptimizerOpen(true)}>
                  <WandSparkles className="h-4 w-4" />
                  {t('actions.optimizePrompt')}
                </Button>
                {canManage ? (
                  <Button onClick={() => setDialogOpen(true)}>
                    <Plus className="h-4 w-4" />
                    {t('actions.newPrompt')}
                  </Button>
                ) : null}
              </div>
            ) : null}
          </div>

          <TabsContent value="library" className="space-y-6">
            {empty ? (
              isOrganizationMode && !canManage ? (
                <div className="rounded-xl border border-dashed p-8 text-center text-muted-foreground">
                  <div className="flex items-center justify-center mb-3">
                    <LibraryBig className="w-8 h-8 text-muted-foreground" />
                  </div>
                  {t('states.empty')}
                </div>
              ) : (
                <div className="rounded-xl border border-dashed p-8 text-center text-muted-foreground">
                  {t('states.empty')}
                </div>
              )
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
                        {grouped[source].map(prompt => (
                          <Link
                            key={prompt.id}
                            href={`/console/prompts/${prompt.id}`}
                            className="rounded-xl border p-4 hover:border-primary/40 hover:bg-muted/20 transition-colors"
                          >
                            <div className="flex items-center gap-2 flex-wrap">
                              <div className="font-medium">{prompt.name}</div>
                              <Badge variant="outline">{prompt.locale}</Badge>
                              <Badge variant="secondary">{prompt.latest_prompt_type}</Badge>
                            </div>
                            <div className="text-sm text-muted-foreground mt-2 line-clamp-2">
                              {prompt.description || t('states.noDescription')}
                            </div>
                            <div className="flex items-center gap-2 flex-wrap mt-3">
                              <Badge variant="outline">v{prompt.latest_version}</Badge>
                              {prompt.latest_labels.map(label => (
                                <Badge key={label} variant={label === 'production' ? 'default' : 'secondary'}>
                                  {label}
                                </Badge>
                              ))}
                            </div>
                          </Link>
                        ))}
                      </div>
                    </section>
                  ) : null
                )}
              </div>
            )}
          </TabsContent>

          <TabsContent value="playground">
            <PromptPlaygroundPanel prefillPromptId={prefillPromptId || undefined} />
          </TabsContent>
        </Tabs>
      </div>

      <PromptFormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={async payload => {
          await createPrompt.mutateAsync(payload);
        }}
      />

      <PromptOptimizerDialog open={optimizerOpen} onOpenChange={setOptimizerOpen} />
    </>
  );
}
