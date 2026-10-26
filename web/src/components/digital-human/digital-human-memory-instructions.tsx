'use client';

import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Eye, Loader2, Play, Plus, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanMemoryPreviewTurn } from '@/services/types/digital-human';
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
  const [previewOpen, setPreviewOpen] = useState(false);
  return (
    <div className="space-y-2">
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
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  digitalHumanId: string;
  instructions: string;
}) {
  const t = useT('digitalHumans');
  const [turns, setTurns] = useState<DigitalHumanMemoryPreviewTurn[]>([
    { user: '', assistant: '' },
  ]);
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
