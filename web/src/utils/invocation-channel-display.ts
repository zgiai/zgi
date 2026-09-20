// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import type { InvocationLogItem } from '../services/types/statistics.ts';

export interface InvocationChannelDisplay {
  isPlatform: boolean;
  providerName: string;
  channelName: string;
  actualChannelName: string;
  supplierName?: string;
}

/** Resolve channel attribution once so list and detail views cannot diverge. */
export function resolveChannelDisplay(
  item: Pick<
    InvocationLogItem,
    'provider_name' | 'channel_name' | 'actual_channel_name' | 'supplier_name' | 'pricing_details'
  > & { use_system_provider?: boolean },
  platformLabel: string
): InvocationChannelDisplay {
  const isPlatform =
    item.pricing_details?.billing_lane === 'platform' ||
    (item.pricing_details?.billing_lane == null && item.use_system_provider === true);
  if (isPlatform) {
    return {
      isPlatform: true,
      providerName: platformLabel,
      channelName: platformLabel,
      actualChannelName: platformLabel,
      supplierName: platformLabel,
    };
  }
  return {
    isPlatform: false,
    providerName: item.provider_name || '-',
    channelName: item.channel_name || '-',
    actualChannelName: item.actual_channel_name || '-',
    supplierName: item.supplier_name || undefined,
  };
}
