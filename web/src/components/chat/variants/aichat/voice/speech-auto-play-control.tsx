'use client';

import { Volume2 } from 'lucide-react';
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

interface AIChatSpeechAutoPlayControlProps {
  enabled: boolean;
  onEnabledChange: (enabled: boolean) => void;
}

export function AIChatSpeechAutoPlayControl({
  enabled,
  onEnabledChange,
}: AIChatSpeechAutoPlayControlProps) {
  const t = useT('webapp');
  const label = t('consoleChat.voice.autoPlay');

  return (
    <div
      className={cn(
        'flex h-8 items-center gap-1.5 rounded-full border border-border/70 bg-background px-2 text-muted-foreground',
        enabled && 'text-foreground'
      )}
      title={label}
    >
      <Volume2 className="size-3.5" aria-hidden="true" />
      <Switch checked={enabled} onCheckedChange={onEnabledChange} aria-label={label} />
    </div>
  );
}
