'use client';

import { useEffect, useMemo, useState } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { RefreshCw, AlertTriangle, ArrowLeft, Home, Copy, Bug } from 'lucide-react';
import * as Sentry from '@sentry/nextjs';

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  const t = useT();
  const [copied, setCopied] = useState(false);

  const diagnostics = useMemo(() => {
    const lines = [
      `message: ${error.message || 'Unknown error'}`,
      error.digest ? `digest: ${error.digest}` : '',
      typeof window !== 'undefined' ? `url: ${window.location.href}` : '',
      typeof navigator !== 'undefined' ? `userAgent: ${navigator.userAgent}` : '',
    ].filter(Boolean);
    return lines.join('\n');
  }, [error]);

  useEffect(() => {
    // Log error to Sentry
    Sentry.captureException(error);
  }, [error]);

  const copyDiagnostics = async () => {
    try {
      await navigator.clipboard.writeText(diagnostics);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  };

  const goBack = () => {
    if (window.history.length > 1) {
      window.history.back();
      return;
    }
    window.location.href = '/console';
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted/20 px-4 py-10">
      <div className="w-full max-w-2xl rounded-lg border bg-background p-6 shadow-sm">
        <div className="flex items-start gap-4">
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-destructive/10">
            <AlertTriangle className="h-6 w-6 text-destructive" />
          </div>
          <div className="min-w-0 flex-1">
            <h1 className="text-xl font-semibold text-foreground">
              {t('common.errorPages.error.title')}
            </h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {t('common.errorPages.error.description')}
            </p>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {t('common.errorPages.error.recoveryHint')}
            </p>
          </div>
        </div>

        <div className="mt-6 rounded-md border bg-muted/30 p-4">
          <div className="mb-2 flex items-center gap-2 text-sm font-medium">
            <Bug className="h-4 w-4" />
            {t('common.errorPages.error.diagnostics')}
          </div>
          <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-words text-xs leading-5 text-muted-foreground">
            {diagnostics}
          </pre>
        </div>

        <div className="mt-6 flex flex-wrap gap-2">
          <Button onClick={reset}>
            <RefreshCw className="mr-2 h-4 w-4" />
            {t('common.errorPages.error.retry')}
          </Button>
          <Button variant="outline" onClick={() => window.location.reload()}>
            <RefreshCw className="mr-2 h-4 w-4" />
            {t('common.errorPages.error.reload')}
          </Button>
          <Button variant="outline" onClick={goBack}>
            <ArrowLeft className="mr-2 h-4 w-4" />
            {t('common.errorPages.error.back')}
          </Button>
          <Button variant="outline" onClick={() => (window.location.href = '/console')}>
            <Home className="mr-2 h-4 w-4" />
            {t('common.errorPages.error.home')}
          </Button>
          <Button variant="ghost" onClick={copyDiagnostics}>
            <Copy className="mr-2 h-4 w-4" />
            {copied ? t('common.errorPages.error.copied') : t('common.errorPages.error.copy')}
          </Button>
        </div>
      </div>
    </div>
  );
}
