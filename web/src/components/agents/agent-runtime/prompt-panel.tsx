'use client';

import { Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface AgentRuntimePromptPanelProps {
  systemPrompt: string;
  className?: string;
  onChangeSystemPrompt: (value: string) => void;
  onOpenOptimizer: () => void;
}

export function AgentRuntimePromptPanel({
  systemPrompt,
  className,
  onChangeSystemPrompt,
  onOpenOptimizer,
}: AgentRuntimePromptPanelProps) {
  const t = useT('agents.agentRuntime');

  return (
    <section className={cn('flex min-w-0 flex-col overflow-hidden', className)}>
      <div className="flex h-12 shrink-0 items-center justify-between px-5">
        <div>
          <h2 className="text-sm font-semibold">{t('prompt.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('prompt.description')}</p>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 px-2 text-xs"
          onClick={onOpenOptimizer}
          disabled={!systemPrompt.trim()}
        >
          <Sparkles className="size-3.5" />
          {t('prompt.optimize')}
        </Button>
      </div>
      <div className="relative min-h-0 flex-1">
        <Textarea
          value={systemPrompt}
          onChange={event => onChangeSystemPrompt(event.target.value)}
          placeholder={t('prompt.placeholder')}
          className="absolute inset-y-0 left-0 right-0 h-full max-h-none resize-none border-0 bg-transparent px-5 pb-6 pt-2 pr-3 text-sm leading-6 shadow-none outline-none scrollbar-thin scrollbar-thumb-muted-foreground/25 scrollbar-track-transparent focus-visible:ring-0"
        />
      </div>
    </section>
  );
}
