'use client';

import { useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Eye, Loader2, Play, Plus, Trash2 } from 'lucide-react';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  DigitalHumanMemoryInstructionsTemplate,
  DigitalHumanMemoryPreviewTurn,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

export const MAX_MEMORY_INSTRUCTIONS = 2000;
const MAX_PREVIEW_TURNS = 10;

interface DigitalHumanMemoryInstructionsProps {
  digitalHumanId: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
}

// Edits the operator's memory instructions (ADR 0015): business guidance for
// what the DigitalHuman should remember or ignore. They never override the
// fixed rules on evidence, sensitive content, or who may see memory.
export function DigitalHumanMemoryInstructions({
  digitalHumanId,
  value,
  onChange,
  disabled,
}: DigitalHumanMemoryInstructionsProps) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [templateId, setTemplateId] = useState('');
  const [pendingTemplate, setPendingTemplate] =
    useState<DigitalHumanMemoryInstructionsTemplate | null>(null);
  const templatesQuery = useQuery({
    queryKey: ['digital-human-memory-templates', locale],
    queryFn: () => digitalHumanService.listMemoryInstructionsTemplates(locale),
    staleTime: 10 * 60 * 1000,
  });
  const templates = templatesQuery.data?.data?.templates ?? [];
  const selectedTemplate = templates.find(template => template.id === templateId);
  // Applying a template replaces the instructions; unsaved text is confirmed
  // first.
  const applyTemplate = (template: DigitalHumanMemoryInstructionsTemplate) => {
    setTemplateId(template.id);
    onChange(template.instructions);
    setPendingTemplate(null);
  };
  return (
    <div className="space-y-2">
      {templates.length > 0 ? (
        <Select
          value={templateId}
          disabled={disabled}
          onValueChange={id => {
            const template = templates.find(item => item.id === id);
            if (!template) return;
            if (value.trim() && value.trim() !== template.instructions) {
              setPendingTemplate(template);
            } else {
              applyTemplate(template);
            }
          }}
        >
          <SelectTrigger aria-label={t('memoryInstructions.template.label')}>
            <SelectValue placeholder={t('memoryInstructions.template.placeholder')} />
          </SelectTrigger>
          <SelectContent>
            {templates.map(template => (
              <SelectItem key={template.id} value={template.id}>
                {template.name} · {template.description}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}
      <ConfirmDialog
        open={Boolean(pendingTemplate)}
        onOpenChange={next => !next && setPendingTemplate(null)}
        title={t('memoryInstructions.template.replaceTitle')}
        description={t('memoryInstructions.template.replaceDescription')}
        confirmText={t('memoryInstructions.template.replace')}
        cancelText={t('common.cancel')}
        variant="warning"
        onConfirm={() => pendingTemplate && applyTemplate(pendingTemplate)}
      />
      <Textarea
        value={value}
        maxLength={MAX_MEMORY_INSTRUCTIONS}
        disabled={disabled}
        className="min-h-28"
        placeholder={t('memoryInstructions.placeholder')}
        onChange={event => onChange(event.target.value)}
      />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {t('memoryInstructions.count', { count: value.length, max: MAX_MEMORY_INSTRUCTIONS })}
        </span>
        <Button type="button" variant="outline" size="sm" onClick={() => setPreviewOpen(true)}>
          <Eye className="size-4" />
          {t('memoryInstructions.preview.open')}
        </Button>
      </div>
      <DigitalHumanMemoryPreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        digitalHumanId={digitalHumanId}
        instructions={value}
        samples={(selectedTemplate ? [selectedTemplate] : templates).flatMap(template =>
          template.samples.map((sample, index) => ({
            id: `${template.id}:${index}`,
            label: `${template.name} · ${sample.name}`,
            turns: sample.turns,
          }))
        )}
      />
    </div>
  );
}

// Runs the background memory proposer over a sample conversation with the
// unsaved instructions and shows what it would keep. Nothing is saved.
function DigitalHumanMemoryPreviewDialog({
  open,
  onOpenChange,
  digitalHumanId,
  instructions,
  samples,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  digitalHumanId: string;
  instructions: string;
  samples: Array<{ id: string; label: string; turns: DigitalHumanMemoryPreviewTurn[] }>;
}) {
  const t = useT('digitalHumans');
  const [turns, setTurns] = useState<DigitalHumanMemoryPreviewTurn[]>([
    { user: '', assistant: '' },
  ]);
  const [sample, setSample] = useState('');
  const preview = useMutation({
    mutationFn: async () =>
      (
        await digitalHumanService.previewMemory(digitalHumanId, {
          instructions,
          turns: turns
            .map(turn => ({ user: turn.user.trim(), assistant: turn.assistant.trim() }))
            .filter(turn => turn.user),
        })
      ).data,
  });
  const canRun = turns.some(turn => turn.user.trim());
  const updateTurn = (index: number, patch: Partial<DigitalHumanMemoryPreviewTurn>) =>
    setTurns(current => current.map((turn, i) => (i === index ? { ...turn, ...patch } : turn)));
  const result = preview.data;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('memoryInstructions.preview.title')}</DialogTitle>
          <DialogDescription>{t('memoryInstructions.preview.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {samples.length > 0 ? (
            <Select
              value={sample}
              onValueChange={id => {
                const picked = samples.find(item => item.id === id);
                if (!picked) return;
                setSample(id);
                setTurns(picked.turns.map(turn => ({ ...turn })));
                preview.reset();
              }}
            >
              <SelectTrigger aria-label={t('memoryInstructions.preview.sample')}>
                <SelectValue placeholder={t('memoryInstructions.preview.samplePlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {samples.map(item => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          <div className="space-y-3">
            {turns.map((turn, index) => (
              <div key={index} className="space-y-2 rounded-md border p-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-xs font-medium text-muted-foreground">
                    {t('memoryInstructions.preview.turn', { index: index + 1 })}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    isIcon
                    aria-label={t('memoryInstructions.preview.removeTurn')}
                    disabled={turns.length <= 1}
                    onClick={() => setTurns(current => current.filter((_, i) => i !== index))}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
                <Textarea
                  value={turn.user}
                  className="min-h-16"
                  placeholder={t('memoryInstructions.preview.userPlaceholder')}
                  onChange={event => updateTurn(index, { user: event.target.value })}
                />
                <Textarea
                  value={turn.assistant}
                  className="min-h-12"
                  placeholder={t('memoryInstructions.preview.assistantPlaceholder')}
                  onChange={event => updateTurn(index, { assistant: event.target.value })}
                />
              </div>
            ))}
          </div>
          <div className="flex flex-wrap justify-between gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={turns.length >= MAX_PREVIEW_TURNS}
              onClick={() => setTurns(current => [...current, { user: '', assistant: '' }])}
            >
              <Plus className="size-4" />
              {t('memoryInstructions.preview.addTurn')}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!canRun || preview.isPending}
              onClick={() => preview.mutate()}
            >
              {preview.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Play className="size-4" />
              )}
              {t('memoryInstructions.preview.run')}
            </Button>
          </div>
          {preview.isError ? (
            <div className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {getErrorMessage(preview.error)}
            </div>
          ) : null}
          {result ? (
            result.proposals.length === 0 ? (
              <div className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
                {t('memoryInstructions.preview.noProposals')}
              </div>
            ) : (
              <div className="space-y-2">
                {result.proposals.map((proposal, index) => (
                  <div key={index} className="space-y-1 rounded-md border p-3 text-xs">
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge variant={proposal.accepted ? 'success' : 'warning'}>
                        {proposal.accepted
                          ? t('memoryInstructions.preview.accepted')
                          : t('memoryInstructions.preview.rejected')}
                      </Badge>
                      <Badge variant="outline">{t(`memory.type.${proposal.memory_type}`)}</Badge>
                      <code className="text-muted-foreground">/me/{proposal.key}.md</code>
                    </div>
                    <p className="whitespace-pre-wrap break-words text-sm">{proposal.content}</p>
                    {proposal.reason ? (
                      <p className="text-muted-foreground">
                        {t('memoryInstructions.preview.reason', { reason: proposal.reason })}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>
            )
          ) : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
