'use client';

// Local caching for system setup status
// English comments only. Strict TS.

import type { SystemSetupStatus } from '@/services/types/setup';

const SETUP_LOCAL_KEY = 'zgi:setup:status' as const;

export function getLocalSetupStatus(): SystemSetupStatus | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = window.localStorage.getItem(SETUP_LOCAL_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as unknown;
    if (parsed && typeof parsed === 'object' && 'step' in (parsed as Record<string, unknown>)) {
      const step = (parsed as { step?: unknown }).step;
      if (step === 'finished' || step === 'not_started') {
        return parsed as SystemSetupStatus;
      }
    }
    return null;
  } catch {
    return null;
  }
}

export function isSetupFinishedLocally(): boolean {
  const status = getLocalSetupStatus();
  return status?.step === 'finished';
}

export function saveSetupFinished(setupAt?: string): SystemSetupStatus {
  const status: SystemSetupStatus = { step: 'finished', ...(setupAt ? { setup_at: setupAt } : {}) };
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(SETUP_LOCAL_KEY, JSON.stringify(status));
    } catch {
      // ignore persistence failure
    }
  }
  return status;
}

export function saveSetupNotStarted(): SystemSetupStatus {
  const status: SystemSetupStatus = { step: 'not_started' };
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.setItem(SETUP_LOCAL_KEY, JSON.stringify(status));
    } catch {
      // ignore persistence failure
    }
  }
  return status;
}

export function clearLocalSetupStatus(): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.removeItem(SETUP_LOCAL_KEY);
  } catch {
    // ignore
  }
}
