'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Eye, FlaskConical, Sparkles } from 'lucide-react';
import { toast } from 'sonner';
import { InfoHint } from '@/components/memory/info-hint';
import { Badge } from '@/components/ui/badge';
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
  onOpenMemoryValues: () => void;
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
  onOpenMemoryValues,
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
  const automaticAvailable = templatesQuery.data?.data?.automatic_extraction_available !== false;

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
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <div id="agent-memory-enabled" className="text-sm font-medium">
                {t('memory.agentTitle')}
              </div>
              <div className="text-xs text-muted-foreground">{t('memory.agentDescription')}</div>
            </div>
            <Switch
              aria-labelledby="agent-memory-enabled"
              checked={agentMemoryEnabled}
              disabled={readOnly}
              onCheckedChange={onChangeAgentMemoryEnabled}
            />
          </div>
          {agentMemoryEnabled && (
            <>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span id="agent-memory-auto" className="text-sm font-medium">
                      {t('memory.autoExtractionTitle')}
                    </span>
                    <InfoHint text={t('memory.autoExtractionDescription')} />
                    {!automaticAvailable && (
                      <Badge variant="warning" title={t('memory.autoUnavailable')}>
                        {t('memory.autoUnavailableBadge')}
                      </Badge>
                    )}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {automaticAvailable
                      ? t('memory.autoExtractionSummary')
                      : t('memory.autoUnavailable')}
                  </div>
                </div>
                <Switch
                  aria-labelledby="agent-memory-auto"
                  checked={agentMemoryAutoExtractionEnabled}
                  disabled={readOnly}
                  onCheckedChange={onChangeAgentMemoryAutoExtractionEnabled}
                />
              </div>
              <div className="space-y-1.5">
                <div className="flex items-center gap-1.5">
                  <span className="text-sm font-medium">{t('memory.instructionsLabel')}</span>
                  <span className="text-xs text-muted-foreground">
                    {t('memory.instructionsOptional')}
                  </span>
                  <InfoHint text={t('memory.instructionsHelp')} />
                </div>
                <Textarea
                  value={agentMemoryInstructions}
                  maxLength={AGENT_MEMORY_LIMITS.maxInstructionsChars}
                  showCharacterCount
                  className="min-h-24"
                  aria-label={t('memory.instructionsLabel')}
                  placeholder={t('memory.instructionsPlaceholder')}
                  disabled={readOnly}
                  onChange={event =>
                    onChangeAgentMemoryInstructions(
                      event.target.value.slice(0, AGENT_MEMORY_LIMITS.maxInstructionsChars)
                    )
                  }
                />
                <div className="flex flex-wrap items-center gap-1.5">
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
                {!agentMemoryInstructions.trim() && (
                  <div className="text-xs text-muted-foreground">
                    {t('memory.instructionsEmpty')}
                  </div>
                )}
              </div>
              <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 border-t pt-2.5">
                <span className="text-xs text-muted-foreground">{t('memory.manageHint')}</span>
                <Button
                  type="button"
                  variant="link"
                  size="xs"
                  className="h-auto px-0 text-primary"
                  onClick={onOpenMemoryValues}
                >
                  <Eye className="size-3" />
                  {t('memory.viewValues')}
                </Button>
              </div>
            </>
          )}
        </div>
      </RuntimeSection>
    </>
  );
}
