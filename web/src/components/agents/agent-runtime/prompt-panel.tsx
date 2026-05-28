'use client';

import { useMemo, useRef, useState } from 'react';
import { Database, FileText, Sparkles, WandSparkles, Wrench } from 'lucide-react';
import WorkflowValueEditor, {
  type VarOption,
  type WorkflowValueEditorHandle,
} from '@/components/workflow/common/workflow-value-editor';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { Dataset } from '@/services/types/dataset';

interface AgentRuntimePromptPanelProps {
  systemPrompt: string;
  className?: string;
  selectedKnowledgeDatasets: Dataset[];
  selectedSkills: AIChatSkillMetadata[];
  onChangeSystemPrompt: (value: string) => void;
  onOpenOptimizer: () => void;
}

const PROMPT_TEMPLATE_KEYS = [
  'generalAssistant',
  'knowledgeQa',
  'customerSupport',
  'toolAssistant',
  'fileAssistant',
  'conciseExpert',
  'internalKnowledge',
  'processGuide',
] as const;

type PromptTemplateKey = (typeof PROMPT_TEMPLATE_KEYS)[number];

const PROMPT_TEMPLATE_CATEGORY_KEYS: Record<PromptTemplateKey, string> = {
  generalAssistant: 'general',
  knowledgeQa: 'knowledge',
  customerSupport: 'service',
  toolAssistant: 'tool',
  fileAssistant: 'tool',
  conciseExpert: 'expert',
  internalKnowledge: 'knowledge',
  processGuide: 'process',
};

export function AgentRuntimePromptPanel({
  systemPrompt,
  className,
  selectedKnowledgeDatasets,
  selectedSkills,
  onChangeSystemPrompt,
  onOpenOptimizer,
}: AgentRuntimePromptPanelProps) {
  const t = useT('agents.agentRuntime');
  const { locale } = useLocale();
  const editorRef = useRef<WorkflowValueEditorHandle | null>(null);
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [selectedTemplateKey, setSelectedTemplateKey] =
    useState<PromptTemplateKey>('knowledgeQa');

  const skillDisplays = useMemo(
    () =>
      selectedSkills.map(skill => ({
        skill,
        display: getAIChatSkillDisplayInfo(skill, locale),
      })),
    [locale, selectedSkills]
  );

  const skillCapabilityItems = useMemo<VarOption[]>(
    () =>
      skillDisplays.map(({ skill, display }) => ({
        sourceId: 'skill',
        sourceTitle: t('prompt.variables.skill'),
        key: skill.skill_id,
        label: display.label || skill.skill_id,
        type: 'object',
        showType: false,
        description: display.description || skill.description || skill.skill_id,
      })),
    [skillDisplays, t]
  );

  const knowledgeCapabilityItems = useMemo<VarOption[]>(
    () =>
      selectedKnowledgeDatasets.map(dataset => ({
        sourceId: 'knowledge',
        sourceTitle: t('prompt.variables.knowledge'),
        key: dataset.id,
        label: dataset.name || dataset.id,
        type: 'object',
        showType: false,
        description: dataset.description || dataset.id,
      })),
    [selectedKnowledgeDatasets, t]
  );

  const capabilityGroups = useMemo(
    () => [
      {
        id: 'agent-skill',
        title: t('prompt.variables.skill'),
        items: skillCapabilityItems,
      },
      {
        id: 'agent-knowledge',
        title: t('prompt.variables.knowledge'),
        items: knowledgeCapabilityItems,
      },
    ],
    [knowledgeCapabilityItems, skillCapabilityItems, t]
  );

  const promptTemplates = useMemo(
    () =>
      PROMPT_TEMPLATE_KEYS.map(key => ({
        key,
        title: t(`prompt.templateLabels.${key}` as never),
        description: t(`prompt.templateDescriptions.${key}` as never),
        category: t(
          `prompt.templateCategories.${PROMPT_TEMPLATE_CATEGORY_KEYS[key]}` as never
        ),
        prompt: t(`prompt.templates.${key}` as never),
      })),
    [t]
  );

  const isPromptEmpty = !systemPrompt.trim();
  const selectedTemplate =
    promptTemplates.find(template => template.key === selectedTemplateKey) ?? promptTemplates[0];

  const insertToken = (sourceId: string, key: string, label: string) => {
    editorRef.current?.insertToken(sourceId, key, label);
  };

  const openTemplateDialog = (key?: PromptTemplateKey) => {
    if (key) {
      setSelectedTemplateKey(key);
    }
    setTemplateDialogOpen(true);
  };

  const applyPromptTemplate = (template: string) => {
    if (editorRef.current) {
      editorRef.current.replaceValue(template);
    } else {
      onChangeSystemPrompt(template);
    }
    setTemplateDialogOpen(false);
  };

  return (
    <section className={cn('flex min-w-0 flex-col overflow-hidden', className)}>
      <div className="flex h-12 shrink-0 items-center justify-between gap-3 px-5">
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold">{t('prompt.title')}</h2>
          {t('prompt.description') ? (
            <p className="truncate text-xs text-muted-foreground">{t('prompt.description')}</p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className="h-8 gap-1.5 px-2 text-xs">
                <Database className="size-3.5" />
                {t('prompt.insertCapability')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-72">
              <DropdownMenuLabel>{t('prompt.variables.skill')}</DropdownMenuLabel>
              {skillDisplays.length === 0 ? (
                <DropdownMenuItem disabled>{t('prompt.variables.noSkill')}</DropdownMenuItem>
              ) : null}
              {skillDisplays.map(({ skill, display }) => (
                <DropdownMenuItem
                  key={skill.skill_id}
                  onSelect={() => insertToken('skill', skill.skill_id, display.label)}
                >
                  <Wrench className="size-4" />
                  <span className="truncate">{display.label}</span>
                </DropdownMenuItem>
              ))}
              <DropdownMenuSeparator />
              <DropdownMenuLabel>{t('prompt.variables.knowledge')}</DropdownMenuLabel>
              {selectedKnowledgeDatasets.length === 0 ? (
                <DropdownMenuItem disabled>{t('prompt.variables.noKnowledge')}</DropdownMenuItem>
              ) : null}
              {selectedKnowledgeDatasets.map(dataset => (
                <DropdownMenuItem
                  key={dataset.id}
                  onSelect={() => insertToken('knowledge', dataset.id, dataset.name)}
                >
                  <Database className="size-4" />
                  <span className="truncate">{dataset.name}</span>
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>

          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 px-2 text-xs"
            onClick={() => openTemplateDialog()}
          >
            <WandSparkles className="size-3.5" />
            {t('prompt.usePromptTemplate')}
          </Button>

          <Button
            variant="ghost"
            size="sm"
            className="h-8 gap-1.5 px-2 text-xs"
            onClick={onOpenOptimizer}
            disabled={!systemPrompt.trim()}
          >
            <Sparkles className="size-3.5" />
            {t('prompt.optimize')}
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1 px-5 pb-5 pt-2">
        <div className="flex h-full min-h-0 flex-col gap-3">
          {isPromptEmpty ? (
            <div className="shrink-0 rounded-md border border-dashed bg-muted/20 p-3">
              <div className="mb-3">
                <div className="text-sm font-semibold">{t('prompt.templatePicker.title')}</div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {t('prompt.templatePicker.description')}
                </div>
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                {promptTemplates.map(template => (
                  <button
                    key={template.key}
                    type="button"
                    className="rounded-md border bg-background p-3 text-left transition-colors hover:border-primary/50 hover:bg-accent focus-ring"
                    onClick={() => openTemplateDialog(template.key)}
                  >
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <FileText className="size-4 text-primary" />
                      <span className="truncate">{template.title}</span>
                    </div>
                    <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                      {template.description}
                    </div>
                  </button>
                ))}
              </div>
            </div>
          ) : null}
          <WorkflowValueEditor
            ref={editorRef}
            value={systemPrompt}
            onChange={onChangeSystemPrompt}
            placeholder={t('prompt.placeholder')}
            emptyBlockPlaceholder={t('prompt.emptyBlockPlaceholder')}
            extraSuggestGroups={capabilityGroups}
            showCharacterCount
            maxLength={20000}
            templateBlocksEnabled
            className="min-h-0 flex-1"
            editorClassName="h-full min-h-full rounded-none border-0 bg-transparent px-0 py-0 shadow-none focus-within:ring-0 [&_.ProseMirror]:min-h-full [&_.ProseMirror]:px-0 [&_.ProseMirror]:py-0 [&_.ProseMirror]:text-sm [&_.ProseMirror]:leading-6"
          />
        </div>
      </div>
      <Dialog open={templateDialogOpen} onOpenChange={setTemplateDialogOpen}>
        <DialogContent size="xl" className="p-0">
          <DialogHeader className="border-b">
            <DialogTitle>{t('prompt.templateDialog.title')}</DialogTitle>
            <DialogDescription>{t('prompt.templateDialog.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="grid min-h-0 gap-4 p-0 md:grid-cols-[280px_minmax(0,1fr)]">
            <div className="max-h-[68vh] overflow-y-auto border-r p-4">
              <div className="mb-3 text-xs font-medium text-muted-foreground">
                {t('prompt.templateDialog.listTitle')}
              </div>
              <div className="space-y-2">
                {promptTemplates.map(template => {
                  const selected = selectedTemplate?.key === template.key;
                  return (
                    <button
                      key={template.key}
                      type="button"
                      className={cn(
                        'w-full rounded-md border p-3 text-left transition-colors focus-ring',
                        selected
                          ? 'border-primary bg-primary/5'
                          : 'bg-background hover:border-primary/40 hover:bg-accent'
                      )}
                      onClick={() => setSelectedTemplateKey(template.key)}
                    >
                      <div className="flex items-start justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-semibold">{template.title}</div>
                          <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                            {template.description}
                          </div>
                        </div>
                        <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
                          {template.category}
                        </span>
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
            <div className="flex max-h-[68vh] min-h-0 flex-col p-4">
              {selectedTemplate ? (
                <>
                  <div className="mb-3">
                    <div className="flex items-center gap-2">
                      <FileText className="size-4 text-primary" />
                      <div className="text-sm font-semibold">{selectedTemplate.title}</div>
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {selectedTemplate.description}
                    </div>
                  </div>
                  {!isPromptEmpty ? (
                    <div className="mb-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700">
                      {t('prompt.templateDialog.replaceWarning')}
                    </div>
                  ) : null}
                  <WorkflowValueEditor
                    value={selectedTemplate.prompt}
                    onChange={() => {}}
                    readOnly
                    suggestEnabled={false}
                    slashTriggerEnabled={false}
                    templateBlocksEnabled
                    className="min-h-0 flex-1"
                    editorClassName="h-full min-h-full rounded-md bg-muted/30 p-4 text-xs leading-5 text-foreground shadow-none focus-within:ring-0 [&_.ProseMirror]:text-xs [&_.ProseMirror]:leading-5"
                  />
                </>
              ) : null}
            </div>
          </DialogBody>
          <DialogFooter className="border-t">
            <Button variant="ghost" onClick={() => setTemplateDialogOpen(false)}>
              {t('prompt.templateDialog.cancel')}
            </Button>
            <Button
              onClick={() => selectedTemplate && applyPromptTemplate(selectedTemplate.prompt)}
              disabled={!selectedTemplate}
            >
              {t('prompt.templateDialog.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
