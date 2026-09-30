'use client';

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ChevronRight } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { mailSettingsURL } from '../setup/mail-setup-state';
import { useT } from '@/i18n';
import type { ConnectorMailEvidence } from '@/services/types/connector';

export function ConnectorMailInformation({
  evidence,
  provider = evidence?.provider ?? 'gmail',
}: {
  evidence?: ConnectorMailEvidence;
  provider?: string;
}) {
  const t = useT('connectors');
  if (evidence) {
    return evidence.usage === 'read_send' ? null : (
      <p className="text-sm text-muted-foreground">
        {t('mail.currentUsage', { usage: t(`mail.${evidence.usage}`) })}
      </p>
    );
  }
  const authorizationCodeHint =
    provider === 'qqmail'
      ? 'mail.qqPasswordHint'
      : provider === 'netease163'
        ? 'mail.neteasePasswordHint'
        : 'mail.authorizationCodeHint';
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {t(provider === 'gmail' ? 'mail.passwordHint' : authorizationCodeHint)}
      </p>
      <a
        className="text-xs text-primary underline underline-offset-4"
        href={mailSettingsURL(provider)}
        target="_blank"
        rel="noopener noreferrer"
      >
        {t(provider === 'gmail' ? 'mail.getPassword' : 'mail.getAuthorizationCode')}
      </a>
      {provider === 'gmail' && (
        <Collapsible className="text-xs text-muted-foreground">
          <CollapsibleTrigger asChild>
            <Button
              variant="ghost"
              className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-xs"
            >
              <ChevronRight
                aria-hidden="true"
                className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
              />
              <span className="min-w-0">{t('mail.eligibilityTitle')}</span>
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="space-y-3">
            <p className="mt-2 leading-5">{t('mail.eligibility')}</p>
            <a
              className="mt-1 inline-block text-primary underline"
              href="https://support.google.com/accounts/answer/185833"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('mail.officialHelp')}
            </a>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
}
