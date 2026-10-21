'use client';

import { AlertTriangle, Info } from 'lucide-react';
import { useLocale } from 'next-intl';
import { OperationResultCard } from '@/components/aichat/operation-cards';
import type { OperationCardMetaItem, OperationCardTextBlock } from '@/components/aichat/operation-cards/types';
import { useT } from '@/i18n/translations';
import { localizeConnectorRuntimeConfirmation } from '@/components/connectors/connector-metadata';
import type {
  ConnectorConfirmationPreview,
  ConnectorRuntimeRequiresConfirmationResult,
} from '@/services/types/connector';

export function ConnectorRuntimePreviewSummary({
  result,
  preview,
  compact = false,
  className,
}: {
  result: ConnectorRuntimeRequiresConfirmationResult;
  preview: ConnectorConfirmationPreview | null;
  compact?: boolean;
  className?: string;
}) {
  const t = useT('connectors');
  const locale = useLocale();
  const confirmation = localizeConnectorRuntimeConfirmation(result.confirmation, locale);
  const metrics: OperationCardMetaItem[] = [];
  const details: OperationCardTextBlock[] = [];

  if (preview?.account_identity) {
    metrics.push({
      id: 'preview-account',
      label: t('runtime.confirmation.previewAccount'),
      value: preview.account_identity,
    });
  }
  preview?.fields.forEach((field, index) => {
    metrics.push({
      id: `preview-field-${index}`,
      label: field.label_i18n[locale] ?? field.label,
      value: field.value,
    });
    if (field.truncated) {
      details.push({
        id: `preview-truncated-${index}`,
        icon: <AlertTriangle className="size-3.5" />,
        title: field.label_i18n[locale] ?? field.label,
        description: t('runtime.confirmation.previewTruncated'),
        tone: 'warning',
      });
    }
  });

  if (!preview || !preview.complete) {
    details.push({
      id: 'preview-incomplete',
      icon: <Info className="size-3.5" />,
      description: t('runtime.confirmation.previewUnavailable'),
      tone: 'warning',
    });
  }

  return (
    <OperationResultCard
      className={className}
      compact={compact}
      status={preview?.complete ? 'info' : 'warning'}
      statusLabel={t('runtime.confirmation.previewEyebrow')}
      eyebrow={t('runtime.presentation.title')}
      title={confirmation.connector_name + ' · ' + confirmation.action_name}
      description={t('runtime.confirmation.description')}
      metrics={metrics}
      details={details}
    />
  );
}
