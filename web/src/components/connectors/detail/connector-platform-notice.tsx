'use client';

import { useT } from '@/i18n';

export function ConnectorPlatformNotice({ connectorId }: { connectorId: string }) {
  const t = useT('connectors');
  if (!['feishu', 'dingtalk', 'wecom', 'weixin'].includes(connectorId)) return null;
  const key = connectorId as 'feishu' | 'dingtalk' | 'wecom' | 'weixin';
  return (
    <div className="space-y-2 rounded-lg border bg-muted/20 p-4 text-sm leading-6">
      <p className="font-medium">{t('platform.capabilityTitle')}</p>
      <p>{t(`platform.${key}`)}</p>
      <p className="text-xs text-muted-foreground">{t('platform.evidenceHint')}</p>
    </div>
  );
}
