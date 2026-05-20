'use client';

import React, { useCallback } from 'react';
import { Button } from '@/components/ui/button';
import { Copy, Trash2 } from 'lucide-react';
import { useT } from '@/i18n';

interface OutputPaneProps {
  streamedText: string;
  finalOutputs?: unknown;
  onClear: () => void;
}

/**
 * Display streamed text and final outputs with copy/clear controls
 */
export const OutputPane: React.FC<OutputPaneProps> = ({ streamedText, finalOutputs, onClear }) => {
  const t = useT();

  const handleCopy = useCallback(() => {
    const content = streamedText || JSON.stringify(finalOutputs, null, 2);
    if (!content) {
      return;
    }
    navigator.clipboard.writeText(content);
  }, [streamedText, finalOutputs]);

  const hasContent = Boolean(streamedText || finalOutputs);

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between px-3 py-2 border-b bg-muted/30">
        <h3 className="text-sm font-medium">{t('webapp.run.output')}</h3>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleCopy}
            disabled={!hasContent}
            className="h-7 px-2"
          >
            <Copy className="w-3.5 h-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={onClear}
            disabled={!hasContent}
            className="h-7 px-2"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </Button>
        </div>
      </div>
      <div className="flex-1 overflow-auto p-3">
        {!hasContent ? (
          <div className="h-full flex items-center justify-center text-sm text-muted-foreground">
            {t('webapp.run.noOutput')}
          </div>
        ) : (
          <div className="space-y-3">
            {streamedText && (
              <div className="text-sm whitespace-pre-wrap break-words">{streamedText}</div>
            )}
            {finalOutputs !== undefined && (
              <div className="border rounded-md p-3 bg-muted/20">
                <pre className="text-xs overflow-auto">
                  {typeof finalOutputs === 'string'
                    ? finalOutputs
                    : JSON.stringify(finalOutputs, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
};
