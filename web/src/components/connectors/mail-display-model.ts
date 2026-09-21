import type { ConnectorCatalogItem } from '@/services/types/connector';

export const MAIL_PROVIDER_IDS = ['qqmail', 'netease163', 'gmail'] as const;
export type MailProviderID = (typeof MAIL_PROVIDER_IDS)[number];
export function isMailProvider(id: string): id is MailProviderID {
  return MAIL_PROVIDER_IDS.some(provider => provider === id);
}
export function mailDisplayKey(id: string): string {
  return isMailProvider(id) ? 'mail' : id;
}

/** Navigation/presentation only. Every action retains the selected real Connector. */
export interface MailDisplayGroup {
  kind: 'mail';
  key: 'mail';
  providers: ConnectorCatalogItem[];
  count: number;
  attentionCount: number;
}
export function mailDisplayGroup(
  items: readonly ConnectorCatalogItem[]
): MailDisplayGroup | undefined {
  const providers = MAIL_PROVIDER_IDS.flatMap(id => items.filter(item => item.id === id));
  if (!providers.length) return undefined;
  return {
    kind: 'mail',
    key: 'mail',
    providers,
    count: providers.reduce((sum, item) => sum + item.connection_summary.count, 0),
    attentionCount: providers.reduce(
      (sum, item) => sum + (item.connection_summary.needs_attention_count ?? 0),
      0
    ),
  };
}
export function selectMailProvider(items: readonly ConnectorCatalogItem[], preferred?: string) {
  return (
    items.find(item => item.id === preferred && isMailProvider(item.id)) ??
    MAIL_PROVIDER_IDS.flatMap(id =>
      items.filter(item => item.id === id && item.setup_availability?.enabled !== false)
    )[0] ??
    MAIL_PROVIDER_IDS.flatMap(id => items.filter(item => item.id === id))[0]
  );
}
