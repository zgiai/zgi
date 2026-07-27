'use client';

import {
  BookOpen,
  ChevronDown,
  Copy,
  ExternalLink,
  Info,
  ListChecks,
  TriangleAlert,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { useT } from '@/i18n';
import type {
  IntegrationAuthSetupGuide,
  IntegrationAuthSetupStep,
} from '@/services/types/integration';
import { useIntegrationMetadata } from './metadata-i18n';

interface AuthSetupGuideProps {
  providerName: string;
  guide: IntegrationAuthSetupGuide | null | undefined;
  callbackURL?: string | null;
}

export function AuthSetupGuide({ providerName, guide, callbackURL }: AuthSetupGuideProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const steps = guide?.steps ?? [];
  if (steps.length === 0) return null;

  const copyCallbackURL = () => {
    if (!callbackURL) return;
    void navigator.clipboard
      .writeText(callbackURL)
      .then(() => toast.success(t('oauth.clientConfig.callbackCopied')))
      .catch(() => toast.error(t('oauth.clientConfig.copyFailed')));
  };

  const renderStepAction = (step: IntegrationAuthSetupStep) => {
    switch (step.action) {
      case 'open_console':
        return guide?.console_url ? (
          <Button asChild size="sm" variant="outline" className="mt-3">
            <a href={guide.console_url} target="_blank" rel="noreferrer noopener">
              <ExternalLink className="size-4" />
              {t('oauth.clientConfig.setupGuide.openConsole', { provider: providerName })}
            </a>
          </Button>
        ) : null;
      case 'open_documentation':
        return guide?.documentation_url ? (
          <Button asChild size="sm" variant="outline" className="mt-3">
            <a href={guide.documentation_url} target="_blank" rel="noreferrer noopener">
              <BookOpen className="size-4" />
              {t('oauth.clientConfig.setupGuide.openDocumentation')}
            </a>
          </Button>
        ) : null;
      case 'copy_callback_url':
        return (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="mt-3"
            disabled={!callbackURL}
            onClick={copyCallbackURL}
          >
            <Copy className="size-4" />
            {t('oauth.clientConfig.copyCallbackURL')}
          </Button>
        );
      default:
        return null;
    }
  };

  return (
    <Collapsible className="overflow-hidden rounded-xl border">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="group flex w-full items-start justify-between gap-4 bg-muted/20 px-4 py-3 text-left transition-colors hover:bg-muted/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
        >
          <span className="flex min-w-0 items-start gap-3">
            <span className="mt-0.5 rounded-lg bg-primary/10 p-2 text-primary">
              <ListChecks className="size-4" />
            </span>
            <span className="min-w-0">
              <span className="block text-sm font-semibold">
                {t('oauth.clientConfig.setupGuide.title', { provider: providerName })}
              </span>
              <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
                {t('oauth.clientConfig.setupGuide.description', { count: steps.length })}
              </span>
            </span>
          </span>
          <span className="mt-1 inline-flex shrink-0 items-center gap-1 text-xs font-medium text-primary">
            {t('oauth.clientConfig.setupGuide.toggle')}
            <ChevronDown className="size-4 transition-transform group-data-[state=open]:rotate-180" />
          </span>
        </button>
      </CollapsibleTrigger>

      <CollapsibleContent>
        <div className="border-t px-4 py-4">
          <ol className="space-y-4">
            {steps.map((step, index) => (
              <li key={step.id} className="grid grid-cols-[1.75rem_minmax(0,1fr)] gap-3">
                <span
                  aria-hidden="true"
                  className="flex size-7 items-center justify-center rounded-full border bg-background text-xs font-semibold text-primary"
                >
                  {index + 1}
                </span>
                <div className="min-w-0 pt-0.5">
                  <p className="text-sm font-medium">
                    {metadata.localizedText(step.title_i18n, step.title)}
                  </p>
                  {step.description ? (
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {metadata.localizedText(step.description_i18n, step.description)}
                    </p>
                  ) : null}
                  {renderStepAction(step)}
                </div>
              </li>
            ))}
          </ol>

          {(guide?.notices?.length ?? 0) > 0 ? (
            <div className="mt-5 space-y-2 border-t pt-4">
              {guide?.notices?.map(notice => {
                const warning = notice.level === 'warning';
                const Icon = warning ? TriangleAlert : Info;
                return (
                  <div
                    key={notice.id}
                    role="note"
                    className={
                      warning
                        ? 'flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/5 px-3 py-2.5 text-warning'
                        : 'flex items-start gap-2 rounded-lg border bg-muted/20 px-3 py-2.5 text-muted-foreground'
                    }
                  >
                    <Icon className="mt-0.5 size-4 shrink-0" />
                    <p className="text-xs leading-5">
                      {metadata.localizedText(notice.text_i18n, notice.text)}
                    </p>
                  </div>
                );
              })}
            </div>
          ) : null}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}
