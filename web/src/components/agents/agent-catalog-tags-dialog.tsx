'use client';

import { useEffect, useMemo, useState } from 'react';
import { Plus, Tag, Tags, X } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import {
  useAgentCatalogTagFacets,
  useAgentCatalogTags,
  useReplaceAgentCatalogTags,
} from '@/hooks/agent/use-agent-catalog-tags';
import { useT } from '@/i18n';
import type { AgentAssetKind, AgentCatalogTagInput } from '@/services/types/agent';

const MAX_TAGS = 8;
const MAX_TAG_LENGTH = 32;

interface AgentCatalogTagsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentId: string;
  workspaceId?: string;
  assetKind: AgentAssetKind;
}

function normalizeTagName(value: string) {
  return value.trim().replace(/\s+/g, ' ');
}

export function AgentCatalogTagsDialog({
  open,
  onOpenChange,
  agentId,
  workspaceId,
  assetKind,
}: AgentCatalogTagsDialogProps) {
  const t = useT('agents.catalogTags');
  const tCommon = useT('common');
  const tagsQuery = useAgentCatalogTags(open ? agentId : null);
  const facetsQuery = useAgentCatalogTagFacets({ workspaceId, assetKind, enabled: open });
  const replaceMutation = useReplaceAgentCatalogTags();
  const [tags, setTags] = useState<AgentCatalogTagInput[]>([]);
  const [input, setInput] = useState('');

  useEffect(() => {
    if (!open || !tagsQuery.data?.data) return;
    setTags(tagsQuery.data.data.items.map(tag => ({ id: tag.id, name: tag.name })));
    setInput('');
  }, [open, tagsQuery.data]);

  const selectedKeys = useMemo(
    () =>
      new Set(
        tags.map(tag =>
          tag.id ? `id:${tag.id}` : `name:${normalizeTagName(tag.name ?? '').toLowerCase()}`
        )
      ),
    [tags]
  );
  const suggestions = facetsQuery.items.filter(
    tag =>
      !selectedKeys.has(`id:${tag.id}`) &&
      !selectedKeys.has(`name:${normalizeTagName(tag.name).toLowerCase()}`)
  );

  const addTag = (tag: AgentCatalogTagInput) => {
    if (tags.length >= MAX_TAGS) {
      toast.error(t('limitReached'));
      return;
    }
    const name = normalizeTagName(tag.name ?? '');
    if (!name) return;
    if ([...name].length > MAX_TAG_LENGTH) {
      toast.error(t('tooLong'));
      return;
    }
    const duplicate = tags.some(
      existing =>
        (tag.id && existing.id === tag.id) ||
        normalizeTagName(existing.name ?? '').toLowerCase() === name.toLowerCase()
    );
    if (duplicate) return;
    setTags(current => [...current, { id: tag.id, name }]);
    setInput('');
  };

  const addInputTag = () => addTag({ name: input });

  const handleSave = async () => {
    await replaceMutation.mutateAsync({
      agentId,
      payload: {
        tags: tags.map(tag =>
          tag.id ? { id: tag.id } : { name: normalizeTagName(tag.name ?? '') }
        ),
      },
    });
    toast.success(t('saveSuccess'));
    onOpenChange(false);
  };

  const isUnavailable = tagsQuery.isLoading || (tagsQuery.isError && !tagsQuery.data?.data);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md" className="overflow-hidden">
        <DialogHeader className="border-b bg-muted/20 px-6 py-5">
          <div className="flex items-start gap-3 pr-8">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary ring-1 ring-primary/10">
              <Tags className="size-5" />
            </div>
            <div className="min-w-0 space-y-1.5">
              <DialogTitle className="text-base leading-5">{t('dialogTitle')}</DialogTitle>
              <DialogDescription className="text-xs leading-5">
                {t('dialogDescription')}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>
        <DialogBody className="space-y-5 px-6 py-5">
          {tagsQuery.isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-9 w-full rounded-lg" />
              <Skeleton className="h-20 w-full rounded-lg" />
            </div>
          ) : tagsQuery.isError && !tagsQuery.data?.data ? (
            <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
              {t('loadError')}
            </p>
          ) : (
            <>
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <label htmlFor={`catalog-tag-input-${agentId}`} className="text-xs font-medium">
                    {t('addLabel')}
                  </label>
                  <span className="text-[11px] tabular-nums text-muted-foreground">
                    {t('selectedCount', { count: tags.length, max: MAX_TAGS })}
                  </span>
                </div>
                <div className="flex gap-2">
                  <Input
                    id={`catalog-tag-input-${agentId}`}
                    value={input}
                    onChange={event => setInput(event.target.value)}
                    onKeyDown={event => {
                      if (event.key === 'Enter') {
                        event.preventDefault();
                        addInputTag();
                      }
                    }}
                    placeholder={t('placeholder')}
                    maxLength={MAX_TAG_LENGTH}
                    disabled={tags.length >= MAX_TAGS}
                    className="h-9 rounded-lg bg-background"
                  />
                  <Button
                    type="button"
                    className="h-9 rounded-lg px-4"
                    onClick={addInputTag}
                    disabled={!normalizeTagName(input) || tags.length >= MAX_TAGS}
                  >
                    <Plus className="size-4" />
                    {t('add')}
                  </Button>
                </div>
              </div>

              <div className="space-y-2">
                <p className="text-xs font-medium">{t('selected')}</p>
                {tags.length > 0 ? (
                  <div className="flex min-h-16 flex-wrap content-start gap-2 rounded-xl border bg-muted/15 p-3">
                    {tags.map((tag, index) => (
                      <Badge
                        key={tag.id ?? `${tag.name}-${index}`}
                        variant="default"
                        className="h-7 gap-1.5 rounded-full border border-primary/15 bg-primary/10 py-1 pl-2.5 pr-1 text-primary"
                      >
                        <Tag className="size-3" />
                        <span className="max-w-48 truncate">{tag.name}</span>
                        <button
                          type="button"
                          className="rounded-full p-1 transition-colors hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          aria-label={t('remove', { name: tag.name ?? '' })}
                          onClick={() =>
                            setTags(current =>
                              current.filter((_, itemIndex) => itemIndex !== index)
                            )
                          }
                        >
                          <X className="size-3" />
                        </button>
                      </Badge>
                    ))}
                  </div>
                ) : (
                  <div className="flex min-h-20 flex-col items-center justify-center gap-2 rounded-xl border border-dashed bg-muted/10 px-4 py-4 text-center">
                    <div className="flex size-8 items-center justify-center rounded-full bg-muted text-muted-foreground">
                      <Tag className="size-4" />
                    </div>
                    <p className="text-xs text-muted-foreground">{t('empty')}</p>
                  </div>
                )}
              </div>

              {facetsQuery.isLoading ? (
                <div className="space-y-2">
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="h-9 w-full rounded-lg" />
                </div>
              ) : suggestions.length > 0 ? (
                <div className="space-y-2.5 border-t pt-4">
                  <p className="text-xs font-medium text-muted-foreground">{t('suggestions')}</p>
                  <div className="flex flex-wrap gap-2">
                    {suggestions.map(tag => (
                      <button
                        key={tag.id}
                        type="button"
                        className="inline-flex h-7 items-center gap-1.5 rounded-full border bg-background px-2.5 text-xs text-muted-foreground transition-colors hover:border-primary/30 hover:bg-primary/5 hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                        onClick={() => addTag(tag)}
                        disabled={tags.length >= MAX_TAGS}
                      >
                        <Plus className="size-3" />
                        <span>{tag.name}</span>
                        <span className="rounded-full bg-muted px-1.5 text-[10px] tabular-nums text-muted-foreground">
                          {tag.count}
                        </span>
                      </button>
                    ))}
                  </div>
                </div>
              ) : null}
            </>
          )}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/15 px-6 py-4">
          <Button
            variant="outline"
            className="min-w-20 rounded-lg"
            onClick={() => onOpenChange(false)}
            disabled={replaceMutation.isPending}
          >
            {tCommon('cancel')}
          </Button>
          <Button
            className="min-w-20 rounded-lg"
            onClick={() => void handleSave()}
            disabled={isUnavailable || replaceMutation.isPending}
          >
            {tCommon('save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
