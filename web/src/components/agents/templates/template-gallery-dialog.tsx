'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useLocale } from 'next-intl';
import { ChevronRight, FilePlus2, Info, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog';
import { useT } from '@/i18n';
import { AGENT_TEMPLATES } from './template-manifest';
import { TemplateCard } from './template-card';
import { TemplatePreview } from './template-preview';
import { TemplateSearchBar } from './template-search-bar';
import { TemplateSidebar } from './template-sidebar';
import {
  getAvailableTemplateKindFilters,
  getCategoryLabel,
  getTemplateCardView,
  getTemplatePreviewView,
  getTemplateSearchText,
  normalizeSearchValue,
  type TemplateTranslator,
} from './template-labels';
import { useCreateAgentFromTemplate } from './use-create-from-template';
import type { AgentTemplate, AgentTemplateCategoryId, AgentTemplateKindFilter } from './types';

interface TemplateGalleryDialogProps {
  open: boolean;
  workspaceId?: string;
  onOpenChange: (open: boolean) => void;
  onCreateBlank: () => void;
  onTemplateCreated?: () => void | Promise<void>;
  initialTemplateId?: string | null;
}

export function TemplateGalleryDialog({
  open,
  workspaceId,
  onOpenChange,
  onCreateBlank,
  onTemplateCreated,
  initialTemplateId,
}: TemplateGalleryDialogProps) {
  const t = useT();
  const templateT = t as TemplateTranslator;
  const locale = useLocale();
  const [activeCategory, setActiveCategory] = useState<AgentTemplateCategoryId>('recommended');
  const [kindFilter, setKindFilter] = useState<AgentTemplateKindFilter>('all');
  const [query, setQuery] = useState('');
  const [previewTemplateId, setPreviewTemplateId] = useState<string | null>(null);
  const [creatingTemplateId, setCreatingTemplateId] = useState<string | null>(null);
  const { createFromTemplate } = useCreateAgentFromTemplate();

  const effectiveWorkspaceId = workspaceId;
  const isCreating = creatingTemplateId !== null;
  const kindOptions = useMemo(() => getAvailableTemplateKindFilters(AGENT_TEMPLATES), []);
  const isCreateDisabled = isCreating || !effectiveWorkspaceId;
  const previewTemplate = useMemo(
    () => AGENT_TEMPLATES.find(template => template.id === previewTemplateId),
    [previewTemplateId]
  );
  const previewView = useMemo(
    () => (previewTemplate ? getTemplatePreviewView(templateT, previewTemplate) : null),
    [previewTemplate, templateT]
  );
  const previewPromptLinks = useMemo(() => {
    if (!previewTemplate?.recommendedPrompts?.length) return [];
    const templateLocale = locale.startsWith('zh') ? 'zh-Hans' : 'en-US';
    return previewTemplate.recommendedPrompts
      .map(reference => {
        const promptId =
          reference.promptIdsByLocale[templateLocale] ??
          reference.promptIdsByLocale['en-US'] ??
          reference.promptIdsByLocale['zh-Hans'];
        if (!promptId) return null;
        return {
          href: `/console/prompts/${promptId}`,
          title: reference.fallbackTitle,
        };
      })
      .filter(Boolean) as Array<{ href: string; title: string }>;
  }, [locale, previewTemplate]);
  const previewLabels = useMemo(
    () => ({
      back: templateT('agents.templates.preview.back'),
      label: templateT('agents.templates.preview.label'),
      overview: templateT('agents.templates.preview.overview'),
      runtimeCheck: templateT('agents.templates.preview.runtimeCheck'),
      dependencies: templateT('agents.templates.preview.dependencies'),
      categories: templateT('agents.templates.preview.categories'),
      recommendedPrompts: templateT('agents.templates.preview.recommendedPrompts'),
      afterCreateTitle: templateT('agents.templates.preview.afterCreateTitle'),
      afterCreateDescription: templateT('agents.templates.preview.afterCreateDescription'),
      readyTitle: templateT('agents.templates.preview.readyTitle'),
      readyDescription: templateT('agents.templates.preview.readyDescription'),
      setupTitle: templateT('agents.templates.preview.setupTitle'),
      setupDescription: templateT('agents.templates.preview.setupDescription'),
      confirm: templateT('agents.templates.preview.confirm'),
    }),
    [templateT]
  );

  const filteredTemplates = useMemo(() => {
    const normalizedQuery = normalizeSearchValue(query);

    return AGENT_TEMPLATES.filter(template => {
      const categoryMatches =
        activeCategory === 'recommended'
          ? template.recommended
          : template.categories.includes(activeCategory);
      if (!categoryMatches) return false;
      if (kindFilter !== 'all' && template.kind !== kindFilter) return false;
      if (!normalizedQuery) return true;

      return getTemplateSearchText(templateT, template).includes(normalizedQuery);
    });
  }, [activeCategory, kindFilter, query, templateT]);
  const hasActiveRefinement = kindFilter !== 'all' || normalizeSearchValue(query).length > 0;

  const resetTemplateNavigation = useCallback(() => {
    setPreviewTemplateId(null);
    setActiveCategory('recommended');
    setKindFilter('all');
    setQuery('');
  }, []);

  const resetTemplateRefinements = useCallback(() => {
    setPreviewTemplateId(null);
    setKindFilter('all');
    setQuery('');
  }, []);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen && isCreating) return;
      if (!nextOpen) {
        resetTemplateNavigation();
      }
      onOpenChange(nextOpen);
    },
    [isCreating, onOpenChange, resetTemplateNavigation]
  );

  const handleCategoryChange = useCallback(
    (category: AgentTemplateCategoryId) => {
      resetTemplateRefinements();
      setActiveCategory(category);
    },
    [resetTemplateRefinements]
  );

  const handleKindFilterChange = useCallback((filter: AgentTemplateKindFilter) => {
    setPreviewTemplateId(null);
    setKindFilter(filter);
  }, []);

  const handleQueryChange = useCallback((nextQuery: string) => {
    setPreviewTemplateId(null);
    setQuery(nextQuery);
  }, []);

  const handleSelectTemplate = useCallback((template: AgentTemplate) => {
    setPreviewTemplateId(template.id);
  }, []);

  const handleClosePreview = useCallback(() => {
    setPreviewTemplateId(null);
  }, []);

  const handleCreateTemplate = useCallback(
    async (template: AgentTemplate) => {
      if (!effectiveWorkspaceId) {
        toast.error(t('agents.validation.workspace.required'));
        return;
      }

      setCreatingTemplateId(template.id);
      try {
        await createFromTemplate(template, effectiveWorkspaceId);
        setPreviewTemplateId(null);
        onOpenChange(false);
        await onTemplateCreated?.();
      } catch (error) {
        console.error('Failed to create agent from template:', error);
      } finally {
        setCreatingTemplateId(null);
      }
    },
    [createFromTemplate, effectiveWorkspaceId, onOpenChange, onTemplateCreated, t]
  );

  useEffect(() => {
    if (!open) return;
    if (!initialTemplateId) return;
    setPreviewTemplateId(initialTemplateId);
  }, [initialTemplateId, open]);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="h-[calc(100vh-2rem)] max-h-[820px] w-[calc(100vw-2rem)] max-w-[1200px] overflow-hidden p-0"
      >
        <div className="flex shrink-0 flex-col gap-3 border-b bg-background px-4 py-3 lg:px-5">
          <div className="flex items-center justify-between gap-4">
            <DialogTitle className="text-base font-semibold">
              {t('agents.templates.title')}
            </DialogTitle>
            <Button
              isIcon
              variant="ghost"
              disabled={isCreating}
              onClick={() => handleOpenChange(false)}
              className="size-8 shrink-0 rounded-lg"
            >
              <X className="size-4" />
            </Button>
          </div>
          <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center lg:pl-[224px]">
            <TemplateSearchBar
              kindFilter={kindFilter}
              kindOptions={kindOptions}
              query={query}
              disabled={isCreating}
              onKindFilterChange={handleKindFilterChange}
              onQueryChange={handleQueryChange}
            />
          </div>
        </div>
        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          <TemplateSidebar
            activeCategory={activeCategory}
            disabled={isCreating}
            onCategoryChange={handleCategoryChange}
          />
          <main className="min-h-0 flex-1 overflow-y-auto bg-muted/10 p-4 lg:p-5">
            {previewTemplate && previewView ? (
              <TemplatePreview
                title={previewView.title}
                description={previewView.description}
                iconLabel={previewTemplate.iconLabel}
                kindLabel={previewView.kindLabel}
                complexityLabel={previewView.complexityLabel}
                runtimeStatusLabel={previewView.runtimeStatusLabel}
                runtimeStatus={previewView.runtimeStatus}
                requirements={previewView.requirements}
                setupRequirements={previewView.setupRequirements}
                categories={previewView.categories}
                runHint={previewView.runHint}
                recommendedPrompts={previewPromptLinks}
                isCreating={creatingTemplateId === previewTemplate.id}
                disabled={isCreating}
                confirmDisabled={isCreateDisabled}
                labels={previewLabels}
                onBack={handleClosePreview}
                onConfirm={() => handleCreateTemplate(previewTemplate)}
              />
            ) : (
              <>
                <button
                  type="button"
                  disabled={isCreating}
                  onClick={onCreateBlank}
                  className="mb-4 flex w-full items-center gap-3 rounded-lg border border-primary/20 bg-background p-3 text-left shadow-sm transition-all hover:border-primary/40 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
                    <FilePlus2 className="size-4" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-semibold text-foreground">
                      {t('agents.templates.createFromBlank')}
                    </span>
                    <span className="mt-0.5 line-clamp-1 block text-xs leading-4 text-muted-foreground">
                      {t('agents.templates.createFromBlankDescription')}
                    </span>
                  </span>
                  <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                </button>
                <div className="mb-3 flex items-center justify-between gap-3">
                  <h2 className="text-sm font-semibold text-foreground">
                    {getCategoryLabel(templateT, activeCategory)}
                  </h2>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {templateT('agents.templates.templateCount', {
                      count: filteredTemplates.length,
                    })}
                  </span>
                </div>
                <div className="mb-4 flex items-start gap-2 rounded-lg border bg-background px-3 py-2 text-xs leading-4 text-muted-foreground">
                  <Info className="mt-0.5 size-3.5 shrink-0" />
                  <span>{templateT('agents.templates.runtimeHint')}</span>
                </div>

                {filteredTemplates.length === 0 ? (
                  <div className="flex min-h-[320px] items-center justify-center rounded-lg border border-dashed bg-muted/20 p-8 text-center">
                    <div>
                      <div className="text-base font-semibold text-foreground">
                        {t('agents.templates.noResults')}
                      </div>
                      <div className="mt-2 text-sm text-muted-foreground">
                        {t('agents.templates.noResultsDescription')}
                      </div>
                      {hasActiveRefinement ? (
                        <Button
                          variant="outline"
                          size="sm"
                          className="mt-4"
                          onClick={resetTemplateRefinements}
                        >
                          {t('agents.templates.clearFilters')}
                        </Button>
                      ) : null}
                    </div>
                  </div>
                ) : (
                  <div className="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3">
                    {filteredTemplates.map(template => {
                      const cardView = getTemplateCardView(templateT, template);

                      return (
                        <TemplateCard
                          key={template.id}
                          template={template}
                          title={cardView.title}
                          description={cardView.description}
                          kindLabel={cardView.kindLabel}
                          complexityLabel={cardView.complexityLabel}
                          runtimeStatusLabel={cardView.runtimeStatusLabel}
                          runtimeStatus={cardView.runtimeStatus}
                          requirementSummary={cardView.requirementSummary}
                          runHint={cardView.runHint}
                          isCreating={creatingTemplateId === template.id}
                          disabled={isCreating}
                          onSelect={handleSelectTemplate}
                        />
                      );
                    })}
                  </div>
                )}
              </>
            )}
          </main>
        </div>
      </DialogContent>
    </Dialog>
  );
}
