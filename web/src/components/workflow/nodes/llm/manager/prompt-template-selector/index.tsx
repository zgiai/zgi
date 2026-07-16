'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
} from '@/components/ui/dropdown-menu';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { Lightbulb, FileText, X, AlertTriangle } from 'lucide-react';
import { Label } from '@/components/ui/label';
import { useTranslations } from 'next-intl';
import { useWorkflowStore } from '@/components/workflow/store';
import { AgentType } from '@/services/types/agent';

// Strict template type to avoid any usage
interface PromptTemplateItem {
  id: string;
  title: string;
  description: string;
  text: string; // Applied to system prompt when selected
}

export interface PromptTemplateSelectorProps {
  className?: string;
  // Called when a template is chosen; apply text to system prompt
  onApply: (text: string) => void;
  disabled?: boolean;
}

type TemplateKey =
  | 'customerService'
  | 'contentCreator'
  | 'dataAnalyst'
  | 'productConsultant'
  // Workflow-oriented
  | 'translation'
  | 'copywriting'
  | 'story'
  | 'codeGeneration';

const PromptTemplateSelector: React.FC<PromptTemplateSelectorProps> = ({
  className,
  onApply,
  disabled = false,
}) => {
  const [open, setOpen] = React.useState(false);
  const [previewTemplate, setPreviewTemplate] = React.useState<PromptTemplateItem | null>(null);
  const t = useTranslations('nodes');
  const agentType = useWorkflowStore.use.agentType();
  const isConversational = agentType === AgentType.CONVERSATIONAL_AGENT;

  const templates: readonly PromptTemplateItem[] = React.useMemo(() => {
    const convKeys: ReadonlyArray<{ key: TemplateKey; id: string }> = [
      { key: 'customerService', id: 'customer-service' },
      { key: 'contentCreator', id: 'content-creator' },
      { key: 'dataAnalyst', id: 'data-analyst' },
      { key: 'productConsultant', id: 'product-consultant' },
    ] as const;
    const workflowKeys: ReadonlyArray<{ key: TemplateKey; id: string }> = [
      { key: 'translation', id: 'translation' },
      { key: 'copywriting', id: 'copywriting' },
      { key: 'story', id: 'story' },
      { key: 'codeGeneration', id: 'code-generation' },
    ] as const;
    const keys = isConversational ? convKeys : workflowKeys;
    return keys.map(({ key, id }) => ({
      id,
      title: t(`llm.promptTemplates.items.${key}.title`),
      description: t(`llm.promptTemplates.items.${key}.description`),
      text: t(`llm.promptTemplates.items.${key}.text`),
    }));
  }, [t, isConversational]);

  const handleTemplateClick = (template: PromptTemplateItem) => {
    setPreviewTemplate(template);
    setOpen(false);
  };

  const handleConfirmApply = () => {
    if (previewTemplate) {
      onApply(previewTemplate.text);
      setPreviewTemplate(null);
    }
  };

  return (
    <>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>
          <span className={cn('inline-flex', className)}>
            <Button
              variant="ghost"
              size="xs"
              isIcon
              className="hover:bg-background"
              aria-label={t('llm.actions.selectQuickPromptTemplate')}
              title={t('llm.actions.selectQuickPromptTemplate')}
              disabled={disabled}
            >
              <Lightbulb className="h-4 w-4" />
            </Button>
          </span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="p-0 w-[340px] rounded-xl" sideOffset={6}>
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b">
            <div className="text-sm font-medium">{t('llm.promptTemplates.title')}</div>
            <button
              type="button"
              className="inline-flex items-center justify-center rounded-md hover:bg-muted p-1"
              aria-label={t('common.cancel')}
              onClick={() => setOpen(false)}
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* List */}
          <div className="p-3 space-y-2">
            {templates.map(item => (
              <button
                key={item.id}
                type="button"
                aria-label={item.title}
                onClick={() => handleTemplateClick(item)}
                className={cn(
                  'w-full text-left rounded-lg border hover:bg-accent hover:text-accent-foreground transition-colors',
                  'px-3 py-2 flex items-start gap-2'
                )}
              >
                <span className="mt-0.5 inline-flex items-center justify-center rounded-md bg-muted text-muted-foreground p-1">
                  <FileText className="h-4 w-4" />
                </span>
                <span className="min-w-0 grow">
                  <div className="text-sm font-medium leading-none mb-1">{item.title}</div>
                  <div className="text-xs text-muted-foreground leading-relaxed">
                    {item.description}
                  </div>
                </span>
              </button>
            ))}
          </div>

          {/* Footer tip */}
          <div className="flex items-center gap-1.5 border-t px-3 py-2 text-xs text-muted-foreground">
            <Lightbulb className="h-3.5 w-3.5" />
            {t('llm.promptTemplates.footerTip')}
          </div>
        </DropdownMenuContent>
      </DropdownMenu>

      {/* Preview Dialog */}
      <Dialog open={!!previewTemplate} onOpenChange={isOpen => !isOpen && setPreviewTemplate(null)}>
        <DialogContent className="max-w-2xl overflow-hidden p-0">
          <DialogHeader className="pb-2">
            <DialogTitle className="flex items-center gap-3 text-xl font-bold tracking-tight">
              <div className="flex h-8 w-8 items-center justify-center rounded bg-primary/10 text-primary">
                <FileText className="h-5 w-5" />
              </div>
              {previewTemplate?.title}
            </DialogTitle>
          </DialogHeader>

          <DialogBody className="space-y-5 py-5 scrollbar-thin">
            <div className="space-y-5">
              {/* Description Section */}
              <div className="space-y-2">
                <div className="flex items-center gap-2 px-1">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('llm.promptTemplates.preview.description')}
                  </Label>
                </div>
                <div className="rounded-lg border bg-muted/20 p-4 text-sm font-medium leading-relaxed text-muted-foreground">
                  {previewTemplate?.description}
                </div>
              </div>

              {/* Preview Content Section */}
              <div className="space-y-2 flex flex-col min-h-0">
                <div className="flex items-center gap-2 px-1">
                  <div className="h-4 w-1 bg-primary rounded-full" />
                  <Label className="text-sm font-bold uppercase tracking-wider text-primary/80">
                    {t('llm.promptTemplates.preview.previewLabel')}
                  </Label>
                </div>
                <div className="flex grow flex-col overflow-hidden rounded-lg border bg-background">
                  <div className="p-5 overflow-y-auto max-h-[36vh] scrollbar-thin">
                    <pre className="text-sm font-mono whitespace-pre-wrap leading-relaxed text-neutral-800 selection:bg-primary/20">
                      {previewTemplate?.text}
                    </pre>
                  </div>
                </div>
              </div>

              {/* Warning Section */}
              <div className="flex items-start gap-3 rounded-lg border bg-muted/25 p-3">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="text-xs font-medium leading-relaxed text-muted-foreground">
                  {t('llm.promptTemplates.preview.warning')}
                </span>
              </div>
            </div>
          </DialogBody>

          <DialogFooter className="border-t bg-muted/20 px-6 pb-5 pt-4">
            <Button
              variant="ghost"
              onClick={() => setPreviewTemplate(null)}
              className="font-semibold"
            >
              {t('llm.promptTemplates.preview.cancel')}
            </Button>
            <Button onClick={handleConfirmApply} size="lg" className="px-10 font-bold shadow-sm">
              {t('llm.promptTemplates.preview.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default PromptTemplateSelector;
