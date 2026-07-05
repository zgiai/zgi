'use client';

import { getUiLocal, updateUiLocal } from '@/utils/ui-local';
import type { UiLocal } from '@/utils/ui-local';

export type ConnectivityStatus = 'success' | 'connectionFailed' | 'connectionTimeout';

export interface ModelLatencyRecord {
  lastMs: number;
  at: number;
  status: ConnectivityStatus;
  error?: string;
  message?: string;
}

export interface ChannelLatencyMap {
  byChannelId: Record<string, Record<string, ModelLatencyRecord>>;
}

function ensureLatencyMap(): ChannelLatencyMap {
  const ui = getUiLocal();
  const existing = ui.channelsLatency;
  if (existing && typeof existing === 'object' && 'byChannelId' in existing) {
    return existing as ChannelLatencyMap;
  }
  const init: ChannelLatencyMap = { byChannelId: {} };
  updateUiLocal({ channelsLatency: init } as Partial<UiLocal>);
  return init;
}

export function getChannelLatencies(channelId: string): Record<string, ModelLatencyRecord> {
  const map = ensureLatencyMap();
  return map.byChannelId[channelId] ?? {};
}

export function getModelLatency(channelId: string, model: string): ModelLatencyRecord | null {
  const map = ensureLatencyMap();
  const ch = map.byChannelId[channelId];
  return ch ? (ch[model] ?? null) : null;
}

export function saveModelLatency(
  channelId: string,
  model: string,
  payload: ModelLatencyRecord
): void {
  const ui = getUiLocal();
  const current: ChannelLatencyMap = ui.channelsLatency ?? { byChannelId: {} };
  const next: ChannelLatencyMap = {
    byChannelId: {
      ...current.byChannelId,
      [channelId]: {
        ...(current.byChannelId[channelId] ?? {}),
        [model]: payload,
      },
    },
  };
  updateUiLocal({ channelsLatency: next } as Partial<UiLocal>);
}

export function removeModelLatencies(channelId: string, models: string[]): void {
  if (!channelId || models.length === 0) return;
  const ui = getUiLocal();
  const current: ChannelLatencyMap = ui.channelsLatency ?? { byChannelId: {} };
  const channelRecords = { ...(current.byChannelId[channelId] ?? {}) };
  models.forEach(model => {
    delete channelRecords[model];
  });
  updateUiLocal({
    channelsLatency: {
      byChannelId: {
        ...current.byChannelId,
        [channelId]: channelRecords,
      },
    },
  } as Partial<UiLocal>);
}

export function classifyFailure(errorMessage: string | undefined): ConnectivityStatus {
  const msg = (errorMessage || '').toLowerCase();
  if (msg.includes('timeout')) return 'connectionTimeout';
  return 'connectionFailed';
}
