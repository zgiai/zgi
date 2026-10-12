'use client';

import { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';

interface ZgiMarketCodeSampleProps {
  label: string;
  code: string;
}

/** A block of code with its name and a copy button. */
export function ZgiMarketCodeSample({ label, code }: ZgiMarketCodeSampleProps) {
  const t = useT();
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(
    () => () => {
      if (timer.current) {
        clearTimeout(timer.current);
      }
    },
    []
  );
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      if (timer.current) {
        clearTimeout(timer.current);
      }
      timer.current = setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t('market.catalog.detail.usage.copyFailed'));
    }
  };
  return (
    <figure className="overflow-hidden rounded-lg border bg-muted/30">
      <figcaption className="flex items-center justify-between gap-2 border-b bg-muted/50 py-1 pl-3 pr-1">
        <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">{label}</span>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-7 shrink-0 gap-1 px-2 text-xs"
          aria-label={`${t('market.catalog.detail.usage.copy')}: ${label}`}
          onClick={() => void copy()}
        >
          {copied ? (
            <Check className="size-3.5" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
          <span aria-live="polite">
            {copied
              ? t('market.catalog.detail.usage.copied')
              : t('market.catalog.detail.usage.copy')}
          </span>
        </Button>
      </figcaption>
      <pre className="max-h-80 overflow-auto p-3 text-xs leading-relaxed" tabIndex={0}>
        <code className="font-mono">{code}</code>
      </pre>
    </figure>
  );
}
