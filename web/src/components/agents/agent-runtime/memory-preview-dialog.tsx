'use client';

import { useState } from 'react';
import { useMutation } from '@tanstack/react-query';
import { Loader2, Play, Plus, Trash2 } from 'lucide-react';
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
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type {
  AgentMemoryPreviewResult,
  AgentMemoryPreviewTurn,
  AgentMemoryTemplate,
} from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

const MAX_PREVIEW_TURNS = 12;

interface AgentMemoryPreviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentId: string;
  instructions: string;
  templates: AgentMemoryTemplate[];
}

// Runs the background consolidation pass over a sample conversation with the
// unsaved memory configuration and shows what it would save. Nothing is saved.
export function AgentMemoryPreviewDialog({
  open,
  onOpenChange,
  agentId,
  instructions,
  templates,
}: AgentMemoryPreviewDialogProps) {
  const t = useT('agents.agentRuntime');
  const [turns, setTurns] = useState<AgentMemoryPreviewTurn[]>([{ user: '', assistant: '' }]);
  const [sample, setSample] = useState('');
  const samples = templates.flatMap(template =>
    template.samples.map((item, index) => ({
      id: `${template.id}:${index}`,
      label: `${template.name} · ${item.name}`,
      turns: item.turns,
    }))
  );
  const preview = useMutation({
    mutationFn: async () => {
      const response = await agentService.previewAgentMemory(agentId, {
        instructions,
        turns: turns
          .map(turn => ({ user: turn.user.trim(), assistant: turn.assistant?.trim() || undefined }))
          .filter(turn => turn.user),
      });
      return response.data as AgentMemoryPreviewResult;
    },
  });
  const canRun = turns.some(turn => turn.user.trim());

  const updateTurn = (index: number, patch: Partial<AgentMemoryPreviewTurn>) =>
    setTurns(current => current.map((turn, i) => (i === index ? { ...turn, ...patch } : turn)));

  const result = preview.data;
  const changedFiles = result?.files.filter(file => file.before !== file.after) ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('memory.preview.title')}</DialogTitle>
          <DialogDescription>{t('memory.preview.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {samples.length > 0 && (
            <Select
              value={sample}
              onValueChange={value => {
                setSample(value);
                const picked = samples.find(item => item.id === value);
                if (picked) {
                  setTurns(picked.turns.map(turn => ({ ...turn })));
                  preview.reset();
                }
              }}
            >
              <SelectTrigger aria-label={t('memory.preview.sample')}>
                <SelectValue placeholder={t('memory.preview.samplePlaceholder')} />
              </SelectTrigger>
              <SelectContent>
                {samples.map(item => (
                  <SelectItem key={item.id} value={item.id}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <div className="space-y-3">
            {turns.map((turn, index) => (
              <div key={index} className="space-y-2 rounded-md border p-3">
                <div className="flex items-center justify-between gap-2">
                  <div className="text-xs font-medium text-muted-foreground">
                    {t('memory.preview.turn', { index: index + 1 })}
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    isIcon
                    aria-label={t('memory.preview.removeTurn')}
                    disabled={turns.length <= 1}
                    onClick={() => setTurns(current => current.filter((_, i) => i !== index))}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
                <Textarea
                  value={turn.user}
                  className="min-h-16"
                  placeholder={t('memory.preview.userPlaceholder')}
                  onChange={event => updateTurn(index, { user: event.target.value })}
                />
                <Textarea
                  value={turn.assistant ?? ''}
                  className="min-h-12"
                  placeholder={t('memory.preview.assistantPlaceholder')}
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
              {t('memory.preview.addTurn')}
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
              {t('memory.preview.run')}
            </Button>
          </div>
          {preview.isError && (
            <div className="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {getErrorMessage(preview.error)}
            </div>
          )}
          {result && (
            <div className="space-y-3">
              <div className="text-xs text-muted-foreground">
                {t('memory.preview.model', { model: `${result.provider}/${result.model}` })}
              </div>
              {result.edits.length === 0 ? (
                <div className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
                  {t('memory.preview.noEdits')}
                </div>
              ) : (
                <div className="space-y-2">
                  {result.edits.map((edit, index) => (
                    <div key={index} className="space-y-1 rounded-md border p-3 text-xs">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant={edit.status === 'accepted' ? 'success' : 'warning'}>
                          {edit.status === 'accepted'
                            ? t('memory.preview.accepted')
                            : t('memory.preview.rejected')}
                        </Badge>
                        <span className="font-mono text-muted-foreground">
                          {edit.command} {edit.path}
                        </span>
                      </div>
                      <div className="whitespace-pre-wrap break-words">
                        {edit.text || edit.new_text || edit.content || edit.old_text}
                      </div>
                      {edit.evidence && (
                        <div className="text-muted-foreground">
                          {t('memory.preview.evidence', { evidence: edit.evidence })}
                        </div>
                      )}
                      {edit.rejection && (
                        <div className="text-warning">
                          {t('memory.preview.rejection', { reason: edit.rejection })}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
              {changedFiles.map(file => (
                <div key={file.key} className="space-y-1 rounded-md bg-muted/30 p-3 text-xs">
                  <div className="font-mono font-medium">{file.path}</div>
                  <div className="whitespace-pre-wrap break-words text-muted-foreground line-through">
                    {file.before || t('memory.preview.empty')}
                  </div>
                  <div className="whitespace-pre-wrap break-words">{file.after}</div>
                </div>
              ))}
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
