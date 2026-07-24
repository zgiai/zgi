'use client';

import { useMemo } from 'react';
import { AlertTriangle } from 'lucide-react';
import { useT } from '@/i18n/translations';
import type {
  AIChatModelPrecheckWarning as AIChatModelPrecheckWarningItem,
  AIChatModelPrecheckWarningKind,
} from '@/services/types/aichat';

const WARNING_TRANSLATION_KEYS = {
  organization_balance_low: 'organizationBalanceLow',
  workspace_quota_low: 'workspaceQuotaLow',
  private_channel_balance_low: 'privateChannelBalanceLow',
  private_channel_upstream_balance_low: 'privateChannelUpstreamBalanceLow',
  private_channel_upstream_unavailable: 'privateChannelUpstreamUnavailable',
} as const satisfies Record<AIChatModelPrecheckWarningKind, string>;

const UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS = {
  balance_exhausted: 'privateChannelUpstreamBalanceExhausted',
  auth_invalid: 'privateChannelUpstreamInvalidKey',
} as const;

type WarningTranslationKey =
  | (typeof WARNING_TRANSLATION_KEYS)[keyof typeof WARNING_TRANSLATION_KEYS]
  | (typeof UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS)[keyof typeof UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS];

function getWarningTranslationKey(
  warning: AIChatModelPrecheckWarningItem
): WarningTranslationKey {
  if (warning.kind === 'private_channel_upstream_unavailable') {
    const reasonKey = warning.reason as keyof typeof UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS;
    if (reasonKey in UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS) {
      return UPSTREAM_UNAVAILABLE_REASON_TRANSLATION_KEYS[reasonKey];
    }
  }
  return WARNING_TRANSLATION_KEYS[warning.kind];
}

interface AIChatModelPrecheckWarningProps {
  warnings: AIChatModelPrecheckWarningItem[];
}

export function AIChatModelPrecheckWarning({ warnings }: AIChatModelPrecheckWarningProps) {
  const t = useT('webapp');
  const warningTranslationKeys = useMemo(
    () => Array.from(new Set(warnings.map(getWarningTranslationKey))),
    [warnings]
  );

  if (warningTranslationKeys.length === 0) {
    return null;
  }

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-warning-foreground shadow-sm"
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
      <div className="min-w-0 text-xs">
        <div className="font-medium text-foreground">{t('consoleChat.modelPrecheck.title')}</div>
        <ul className="mt-0.5 space-y-0.5 text-muted-foreground">
          {warningTranslationKeys.map(translationKey => (
            <li key={translationKey}>{t(`consoleChat.modelPrecheck.${translationKey}`)}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}
