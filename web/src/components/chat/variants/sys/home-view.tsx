import * as React from 'react';
import { Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';

interface SysHomeViewProps {
  onSuggestionClick: (text: string) => void;
}

export function SysHomeView({ onSuggestionClick }: SysHomeViewProps) {
  const t = useT('webapp');

  return (
    <div className="w-full max-w-3xl flex flex-col items-center gap-8 -mt-20 animate-in fade-in zoom-in duration-500">
      <div className="flex flex-col items-center gap-4">
        <div className="h-16 w-16 bg-highlight/5 rounded-2xl shadow-md shadow-highlight border border-highlight flex items-center justify-center">
          <Sparkles className="h-8 w-8 text-highlight" />
        </div>
        <h1 className="text-2xl font-bold text-foreground">{t('chat.startConversation')}</h1>
        <p className="text-muted-foreground text-sm">{t('chat.chooseAssistant')}</p>
      </div>

      {/* Spacer for InputArea which will be positioned absolutely over this */}
      <div className="w-full h-[140px] shrink-0" />

      <div className="flex flex-wrap items-center justify-center gap-2">
        {[
          { text: t('chat.suggestions.email'), key: 'email' },
          { text: t('chat.suggestions.meeting'), key: 'meeting' },
          { text: t('chat.suggestions.report'), key: 'report' },
          { text: t('chat.suggestions.polish'), key: 'polish' },
        ].map(suggestion => (
          <Button
            key={suggestion.key}
            variant="outline"
            className="rounded-full text-xs font-normal h-8 bg-muted/30 border-transparent hover:bg-muted/50 hover:border-border/50 transition-all"
            onClick={() => onSuggestionClick(suggestion.text)}
          >
            {suggestion.text}
          </Button>
        ))}
      </div>
    </div>
  );
}
