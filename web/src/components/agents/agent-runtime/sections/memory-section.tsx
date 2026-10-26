'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { FlaskConical, Sparkles } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type { AgentMemoryTemplate } from '@/services/types/agent';
import { AgentMemoryPreviewDialog } from '../memory-preview-dialog';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';
import { AGENT_MEMORY_LIMITS } from '../utils';

interface AgentRuntimeMemorySectionProps {
  agentId: string;
  open: boolean;
  agentMemoryEnabled: boolean;
  agentMemoryAutoExtractionEnabled: boolean;
  agentMemoryInstructions: string;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemoryAutoExtractionEnabled: (value: boolean) => void;
  onChangeAgentMemoryInstructions: (value: string) => void;
}

// Agent memory has no configured types (ADR 0017): the builder describes what
// to remember in the memory instructions, and the model organizes each user's
// memory files itself.
export function AgentRuntimeMemorySection({
  agentId,
  open,
  agentMemoryEnabled,
  agentMemoryAutoExtractionEnabled,
  agentMemoryInstructions,
  readOnly = false,
  onToggleSection,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemoryAutoExtractionEnabled,
  onChangeAgentMemoryInstructions,
}: AgentRuntimeMemorySectionProps) {
  const t = useT('agents.agentRuntime');
  const { locale } = useLocale();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [pendingTemplate, setPendingTemplate] = useState<AgentMemoryTemplate | null>(null);
  const templatesQuery = useQuery({
    queryKey: AGENT_KEYS.memoryTemplates(agentId, locale),
    queryFn: () => agentService.getAgentMemoryTemplates(agentId, locale),
    enabled: Boolean(agentId) && open,
    staleTime: 10 * 60 * 1000,
  });
  const memoryTemplates = templatesQuery.data?.data?.catalog.templates ?? [];

  const applyTemplate = (template: AgentMemoryTemplate) => {
    if (readOnly) return;
    onChangeAgentMemoryEnabled(true);
    onChangeAgentMemoryInstructions(
      template.instructions.slice(0, AGENT_MEMORY_LIMITS.maxInstructionsChars)
    );
    setTemplateDialogOpen(false);
    setPendingTemplate(null);
    toast.success(t('memory.templateApplied'));
  };
  const chooseTemplate = (template: AgentMemoryTemplate) => {
    const current = agentMemoryInstructions.trim();
    if (current && current !== template.instructions.trim()) {
      setPendingTemplate(template);
      return;
    }
    applyTemplate(template);
  };

  return (
    <>
      {previewOpen && (
        <AgentMemoryPreviewDialog
          open={previewOpen}
          onOpenChange={setPreviewOpen}
          agentId={agentId}
          instructions={agentMemoryInstructions}
          templates={memoryTemplates}
        />
      )}
      <ConfirmDialog
        open={Boolean(pendingTemplate)}
        onOpenChange={dialogOpen => {
          if (!dialogOpen) setPendingTemplate(null);
        }}
        title={t('memory.templateReplaceConfirmTitle')}
        description={t('memory.templateReplaceConfirmDescription')}
        confirmText={t('memory.templateReplaceConfirmAction')}
        cancelText={t('memory.templateReplaceConfirmCancel')}
        variant="warning"
        onConfirm={() => {
          if (pendingTemplate) applyTemplate(pendingTemplate);
        }}
      />
      <Dialog open={templateDialogOpen} onOpenChange={setTemplateDialogOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('memory.templateDialogTitle')}</DialogTitle>
            <DialogDescription>{t('memory.templateDialogDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            {templatesQuery.isLoading && (
              <div className="text-xs text-muted-foreground">{t('memory.templatesLoading')}</div>
            )}
            {memoryTemplates.map(template => (
              <div key={template.id} className="space-y-3 rounded-md border p-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-semibold">
                    <Sparkles className="size-4 text-primary" />
                    <span>{template.name}</span>
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">{template.description}</div>
                </div>
                <div className="rounded-md bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                  {template.instructions}
                </div>
                <div className="flex justify-end">
                  <Button
                    type="button"
                    size="sm"
                    disabled={readOnly}
                    onClick={() => chooseTemplate(template)}
                  >
                    <Sparkles className="size-4" />
                    {t('memory.templateUse')}
                  </Button>
                </div>
              </div>
            ))}
          </DialogBody>
        </DialogContent>
      </Dialog>

      <RuntimeSection
        title={t('sections.memory')}
        section="memory"
        open={open}
        onToggle={onToggleSection}
      >
        <div className="space-y-3">
          <div className="space-y-3 rounded-md border p-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-medium">{t('memory.agentTitle')}</div>
                <div className="text-xs text-muted-foreground">{t('memory.agentDescription')}</div>
              </div>
              <Switch
                checked={agentMemoryEnabled}
                disabled={readOnly}
                onCheckedChange={onChangeAgentMemoryEnabled}
              />
            </div>
            {agentMemoryEnabled && (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3 rounded-md bg-muted/30 px-3 py-2">
                  <div>
                    <div className="text-xs font-medium">{t('memory.autoExtractionTitle')}</div>
                    <div className="text-[11px] text-muted-foreground">
                      {t('memory.autoExtractionDescription')}
                    </div>
                  </div>
                  <Switch
                    checked={agentMemoryAutoExtractionEnabled}
                    disabled={readOnly}
                    onCheckedChange={onChangeAgentMemoryAutoExtractionEnabled}
                  />
                </div>
                <div className="space-y-1">
                  <div className="flex items-center justify-between gap-2">
                    <div className="text-xs font-medium text-muted-foreground">
                      {t('memory.instructionsLabel')}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={() => setTemplateDialogOpen(true)}
                        disabled={readOnly}
                      >
                        <Sparkles className="size-3" />
                        {t('memory.applyTemplate')}
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={() => setPreviewOpen(true)}
                      >
                        <FlaskConical className="size-3" />
                        {t('memory.preview.open')}
                      </Button>
                    </div>
                  </div>
                  <Textarea
                    value={agentMemoryInstructions}
                    maxLength={AGENT_MEMORY_LIMITS.maxInstructionsChars}
                    showCharacterCount
                    className="min-h-28"
                    placeholder={t('memory.instructionsPlaceholder')}
                    disabled={readOnly}
                    onChange={event =>
                      onChangeAgentMemoryInstructions(
                        event.target.value.slice(0, AGENT_MEMORY_LIMITS.maxInstructionsChars)
                      )
                    }
                  />
                  <div className="text-[11px] text-muted-foreground">
                    {t('memory.instructionsHelp')}
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </RuntimeSection>
    </>
  );
}
