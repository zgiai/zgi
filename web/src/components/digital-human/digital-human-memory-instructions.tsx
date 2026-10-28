'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useMutation, useQuery } from '@tanstack/react-query';
import { ArrowRight, Loader2, MessageSquareText, Play, Plus, Trash2 } from 'lucide-react';
import { MemoryInstructionsField, MemorySettingRow } from '@/components/memory/memory-settings';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import {
  useDigitalHumanMemories,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanMemoryPreviewTurn } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

export const MAX_MEMORY_INSTRUCTIONS = 2000;
const MAX_PREVIEW_TURNS = 10;

/** The parts of a DigitalHuman's memory policy its operator edits. */
export interface DigitalHumanMemorySettingsValue {
  userMemoryEnabled: boolean;
  userAutoCaptureEnabled: boolean;
  instructions: string;
}

// memorySettingsOf reads the operator's memory settings from a memory policy
// (ADR 0015, ADR 0019). Missing switches are on.
export function memorySettingsOf(
  policy?: Record<string, unknown>
): DigitalHumanMemorySettingsValue {
  return {
    userMemoryEnabled: policy?.user_memory_enabled !== false,
    userAutoCaptureEnabled: policy?.user_auto_capture_enabled !== false,
    instructions: typeof policy?.instructions === 'string' ? policy.instructions : '',
  };
}

// withMemorySettings keeps the rest of the memory policy as it is. Switches
// that are on and empty instructions are left out, as the server reads them.
export function withMemorySettings(
  policy: Record<string, unknown> | undefined,
  settings: DigitalHumanMemorySettingsValue
) {
  const rest = { ...(policy ?? {}) };
  delete rest.instructions;
  delete rest.user_memory_enabled;
  delete rest.user_auto_capture_enabled;
  const instructions = settings.instructions.trim();
  return {
    ...rest,
    ...(settings.userMemoryEnabled ? {} : { user_memory_enabled: false }),
    ...(settings.userAutoCaptureEnabled ? {} : { user_auto_capture_enabled: false }),
    ...(instructions ? { instructions } : {}),
  };
}

interface DigitalHumanMemorySettingsProps {
  digitalHumanId: string;
  value: DigitalHumanMemorySettingsValue;
  onChange: (value: DigitalHumanMemorySettingsValue) => void;
  disabled: boolean;
}

// Edits how a DigitalHuman remembers: whether it remembers each person, and
// automatically, the operator's memory instructions, and whether it builds
// up its own work experience. The first three are part of the draft; the
// last is the DigitalHuman's memory state and changes at once.
export function DigitalHumanMemorySettings({
  digitalHumanId,
  value,
  onChange,
  disabled,
}: DigitalHumanMemorySettingsProps) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const permissions = useAccountPermissions();
  const canManageMemory = permissions.hasPermission('digital_human.memory.manage');
  const mutations = useDigitalHumanMutations(digitalHumanId);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [templateId, setTemplateId] = useState('');
  const templatesQuery = useQuery({
    queryKey: ['digital-human-memory-templates', locale],
    queryFn: () => digitalHumanService.listMemoryInstructionsTemplates(locale),
    staleTime: 10 * 60 * 1000,
  });
  const templates = templatesQuery.data?.data?.templates ?? [];
  const selectedTemplate = templates.find(template => template.id === templateId);
  const memoryState = useDigitalHumanMemories(digitalHumanId, { page_size: 1 }, canManageMemory)
    .data?.data?.state;
  const toggleWorkMemory = async (next: boolean) => {
    if (!memoryState) return;
    try {
      await mutations.updateMemoryState.mutateAsync({
        digitalHumanId,
        expectedMemoryEpoch: memoryState.memory_epoch,
        autoExtractionEnabled: next,
      });
    } catch {
      // The mutation already reports the error.
    }
  };
  const samples = (selectedTemplate ? [selectedTemplate] : templates).flatMap(template =>
    template.samples.map((sample, index) => ({
      id: `${template.id}:${index}`,
      label: `${template.name} · ${sample.name}`,
      turns: sample.turns,
    }))
  );

  return (
    <div className="space-y-4">
      <MemorySettingRow
        id="digital-human-user-memory"
        title={t('memoryInstructions.userMemoryTitle')}
        description={t('memoryInstructions.userMemoryDescription')}
        checked={value.userMemoryEnabled}
        disabled={disabled}
        onCheckedChange={checked => onChange({ ...value, userMemoryEnabled: checked })}
      />
      {value.userMemoryEnabled ? (
        <MemorySettingRow
          id="digital-human-user-auto"
          title={t('memoryInstructions.autoTitle')}
          description={t('memoryInstructions.autoDescription')}
          hint={t('memoryInstructions.autoHint')}
          checked={value.userAutoCaptureEnabled}
          disabled={disabled}
          onCheckedChange={checked => onChange({ ...value, userAutoCaptureEnabled: checked })}
        />
      ) : null}
      {canManageMemory ? (
        <MemorySettingRow
          id="digital-human-work-memory"
          title={t('memoryInstructions.workTitle')}
          description={t('memoryInstructions.workDescription')}
          hint={t('memoryInstructions.workHint')}
          badge={
            <Badge variant="outline" className="font-normal">
              {t('memoryInstructions.workImmediate')}
            </Badge>
          }
          checked={memoryState?.auto_extraction_enabled ?? false}
          disabled={!memoryState || mutations.updateMemoryState.isPending}
          onCheckedChange={next => void toggleWorkMemory(next)}
        />
      ) : null}
      <MemoryInstructionsField
        value={value.instructions}
        onChange={instructions => onChange({ ...value, instructions })}
        maxLength={MAX_MEMORY_INSTRUCTIONS}
        templates={templates}
        placeholder={t('memoryInstructions.placeholder')}
        disabled={disabled}
        onTemplateApplied={template => setTemplateId(template.id)}
      />
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t pt-2.5">
        <Button
          type="button"
          variant="link"
          size="xs"
          className="h-auto px-0 text-primary"
          onClick={() => setPreviewOpen(true)}
        >
          <MessageSquareText className="size-3" />
          {t('memoryInstructions.tryPreview')}
        </Button>
        <Button asChild variant="link" size="xs" className="h-auto px-0 text-primary">
          <Link href={`/console/digital-humans/${encodeURIComponent(digitalHumanId)}/memory`}>
            {t('memoryInstructions.viewMemory')}
            <ArrowRight className="size-3" />
          </Link>
        </Button>
      </div>
      {previewOpen ? (
        <DigitalHumanMemoryPreviewDialog
          open
          onOpenChange={setPreviewOpen}
          digitalHumanId={digitalHumanId}
          instructions={value.instructions}
          samples={samples}
        />
      ) : null}
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
  // It opens on the first sample, so there is something to run at once.
  const [turns, setTurns] = useState<DigitalHumanMemoryPreviewTurn[]>(() =>
    samples[0] ? samples[0].turns.map(turn => ({ ...turn })) : [{ user: '', assistant: '' }]
  );
  const [sample, setSample] = useState(samples[0]?.id ?? '');
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
