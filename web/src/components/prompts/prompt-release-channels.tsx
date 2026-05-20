'use client';

import { useEffect, useMemo, useState } from 'react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import type { PromptSource, PromptVersion } from '@/services/types/prompt';

const CHANNEL_NONE = '__none__';

const releaseChannels = [
  { value: 'production', i18nKey: 'production' },
  { value: 'staging', i18nKey: 'staging' },
  { value: 'gray-a', i18nKey: 'grayA' },
  { value: 'gray-b', i18nKey: 'grayB' },
] as const;

interface PromptReleaseChannelsProps {
  versions: PromptVersion[];
  promptSource: PromptSource;
  canManage: boolean;
  isPending?: boolean;
  onSaveLabels: (version: number, labels: string[]) => Promise<unknown> | unknown;
}

function withoutReservedLabels(labels: string[]) {
  return labels.filter(label => label !== 'latest');
}

export function PromptReleaseChannels({
  versions,
  promptSource,
  canManage,
  isPending = false,
  onSaveLabels,
}: PromptReleaseChannelsProps) {
  const t = useT('prompts');
  const isReadOnly = !canManage || promptSource === 'official';

  const assignments = useMemo(() => {
    const next: Record<string, number | null> = {};
    releaseChannels.forEach(channel => {
      const version = versions.find(item => item.labels.includes(channel.value));
      next[channel.value] = version?.version ?? null;
    });
    return next;
  }, [versions]);

  const [drafts, setDrafts] = useState<Record<string, string>>({});

  useEffect(() => {
    const next: Record<string, string> = {};
    releaseChannels.forEach(channel => {
      next[channel.value] = assignments[channel.value]
        ? String(assignments[channel.value])
        : CHANNEL_NONE;
    });
    setDrafts(next);
  }, [assignments]);

  const handleApply = async (channel: (typeof releaseChannels)[number]) => {
    const selected = drafts[channel.value] ?? CHANNEL_NONE;
    const currentVersion = assignments[channel.value];

    if (selected === CHANNEL_NONE) {
      if (!currentVersion) return;
      const sourceVersion = versions.find(item => item.version === currentVersion);
      if (!sourceVersion) return;
      await onSaveLabels(
        sourceVersion.version,
        withoutReservedLabels(sourceVersion.labels).filter(label => label !== channel.value)
      );
      return;
    }

    const targetVersion = versions.find(item => item.version === Number(selected));
    if (!targetVersion) return;

    const labels = Array.from(
      new Set([...withoutReservedLabels(targetVersion.labels), channel.value])
    );

    await onSaveLabels(targetVersion.version, labels);
  };

  return (
    <div className="rounded-xl border p-4 space-y-4">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold">{t('releaseChannels.title')}</h2>
        <p className="text-sm text-muted-foreground">{t('releaseChannels.description')}</p>
      </div>

      {isReadOnly ? (
        <div className="rounded-lg border bg-muted/20 px-4 py-3 text-sm text-muted-foreground">
          {t('releaseChannels.readOnlyHint')}
        </div>
      ) : null}

      <div className="space-y-3">
        {releaseChannels.map(channel => {
          const currentVersion = assignments[channel.value];
          const draftValue = drafts[channel.value] ?? CHANNEL_NONE;
          const isDirty =
            String(currentVersion ?? CHANNEL_NONE) !==
            String(draftValue === CHANNEL_NONE ? CHANNEL_NONE : Number(draftValue));
          const isClearing = draftValue === CHANNEL_NONE && currentVersion != null && isDirty;

          return (
            <div key={channel.value} className="rounded-lg border p-3 space-y-3">
              <div className="flex items-center justify-between gap-3 flex-wrap">
                <div className="space-y-1">
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">
                      {t(`releaseChannels.items.${channel.i18nKey}.label` as const)}
                    </Badge>
                    {currentVersion ? (
                      <Badge variant="secondary">v{currentVersion}</Badge>
                    ) : (
                      <Badge variant="outline">{t('releaseChannels.unassigned')}</Badge>
                    )}
                  </div>
                  <div className="text-sm text-muted-foreground">
                    {t(`releaseChannels.items.${channel.i18nKey}.description` as const)}
                  </div>
                </div>
              </div>

              <div className="flex flex-col md:flex-row gap-2">
                <Select
                  value={draftValue}
                  onValueChange={value =>
                    setDrafts(previous => ({
                      ...previous,
                      [channel.value]: value,
                    }))
                  }
                  disabled={isReadOnly || isPending}
                >
                  <SelectTrigger className="md:max-w-[220px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={CHANNEL_NONE}>{t('releaseChannels.unassigned')}</SelectItem>
                    {versions.map(version => (
                      <SelectItem key={version.id} value={String(version.version)}>
                        v{version.version}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  variant="outline"
                  disabled={isReadOnly || isPending || !isDirty}
                  onClick={() => void handleApply(channel)}
                >
                  {isClearing ? t('releaseChannels.clear') : t('releaseChannels.apply')}
                </Button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export default PromptReleaseChannels;
