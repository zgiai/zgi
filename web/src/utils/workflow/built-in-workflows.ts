'use client';

// Client local caching for built-in workflows with 1-week expiration
// English comments only. Strict TS.

import type { BuiltInWorkflowList, BuiltInWorkflow } from '@/services/types/workflow';
import { deleteCookie, getCookie } from '../cookie';
import {
  BUILT_IN_WORKFLOWS_CLIENT_CACHE_TTL_MS,
  CLIENT_CACHE_KEYS,
  LEGACY_CLIENT_CACHE_COOKIE_KEYS,
  readClientCache,
  removeClientCache,
  writeClientCache,
} from '../client-cache';

const BUILT_IN_WORKFLOWS_CACHE_KEY = CLIENT_CACHE_KEYS.builtInWorkflows;

/**
 * Get cached built-in workflows from client local cache
 * Returns null if cache is not present or expired
 */
export function getCachedBuiltInWorkflows(): BuiltInWorkflowList | null {
  if (typeof window === 'undefined') return null;
  try {
    const local = readClientCache<BuiltInWorkflowList>(BUILT_IN_WORKFLOWS_CACHE_KEY, {
      validate: (value: unknown): value is BuiltInWorkflowList => Array.isArray(value),
    });
    if (local) {
      return local;
    }

    const legacy = getCookie<unknown>(LEGACY_CLIENT_CACHE_COOKIE_KEYS.builtInWorkflows);
    if (
      legacy &&
      typeof legacy === 'object' &&
      'data' in legacy &&
      Array.isArray((legacy as { data?: unknown }).data)
    ) {
      const data = (legacy as { data: BuiltInWorkflowList }).data;
      writeClientCache(BUILT_IN_WORKFLOWS_CACHE_KEY, data, BUILT_IN_WORKFLOWS_CLIENT_CACHE_TTL_MS);
      deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.builtInWorkflows);
      return data;
    }

    return null;
  } catch {
    return null;
  }
}

/**
 * Save built-in workflows to client local cache with 1-week expiration
 */
export function saveBuiltInWorkflows(data: BuiltInWorkflowList): void {
  if (typeof window === 'undefined') return;
  try {
    writeClientCache(BUILT_IN_WORKFLOWS_CACHE_KEY, data, BUILT_IN_WORKFLOWS_CLIENT_CACHE_TTL_MS);
    deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.builtInWorkflows);
  } catch {
    // Ignore cache write failure
  }
}

/**
 * Clear built-in workflows cache
 */
export function clearBuiltInWorkflowsCache(): void {
  if (typeof window === 'undefined') return;
  try {
    removeClientCache(BUILT_IN_WORKFLOWS_CACHE_KEY);
    deleteCookie(LEGACY_CLIENT_CACHE_COOKIE_KEYS.builtInWorkflows);
  } catch {
    // Ignore
  }
}

/**
 * Check if built-in workflows are cached locally
 */
export function hasBuiltInWorkflowsCache(): boolean {
  return getCachedBuiltInWorkflows() !== null;
}

/**
 * Find a built-in workflow by scenario (e.g., 'bi_chat', 'global_chat')
 */
export function findBuiltInWorkflowByScenario(
  workflows: BuiltInWorkflowList,
  scenario: string
): BuiltInWorkflow | undefined {
  return workflows.find(w => w.scenario === scenario);
}
