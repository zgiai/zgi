'use client';

import {
  ContextualAIChatProvider,
  useAIChatContextRegistration as useContextualAIChatRegistration,
  useContextualAIChat as useContextualAIChatBase,
} from '../contextual/contextual-ai-chat-context';
import type { AIChatContextRegistrationOptions } from '../contextual/types';
import type {
  AIChatPageContextItem,
  AIChatPageContextRegistrationOptions,
} from './types';

function toContextualRegistrationOptions(
  options?: AIChatPageContextRegistrationOptions
): AIChatContextRegistrationOptions | undefined {
  if (!options) return undefined;
  return {
    scopeId: options.scopeId,
    replace: options.replace,
    priority: options.priority,
    visibility: options.visibility,
  };
}

export {
  ContextualAIChatProvider,
  ContextualAIChatProvider as AIChatPageContextProvider,
  ContextualAIChatProvider as AIChatPageContextRegistryProvider,
  ContextualAIChatProvider as PageContextProvider,
  ContextualAIChatProvider as PageContextRegistryProvider,
};

export function usePageContext() {
  const context = useContextualAIChatBase();
  return {
    ...context,
    items: context.items as AIChatPageContextItem[],
  };
}

export const useContextualAIChat = usePageContext;
export const usePageContextRegistry = usePageContext;
export const useAIChatPageContext = usePageContext;

export function usePageContextRegistration(
  items: AIChatPageContextItem[],
  options?: AIChatPageContextRegistrationOptions
) {
  useContextualAIChatRegistration(items, toContextualRegistrationOptions(options));
}

export const useAIChatContextRegistration = usePageContextRegistration;
export const useAIChatPageContextRegistration = usePageContextRegistration;
