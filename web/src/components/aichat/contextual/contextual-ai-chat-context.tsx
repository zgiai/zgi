'use client';

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type {
  AIChatCapabilityDescriptor,
  AIChatContextItem,
  AIChatContextRegistrationOptions,
  AIChatContextRegistrationVisibility,
  AIChatContextRelation,
} from './types';

const CONTEXTUAL_AICHAT_OPEN_STORAGE_KEY = 'consoleChat.contextualDockOpen';
const CONTEXTUAL_AICHAT_MEDIA_QUERY = '(min-width: 1024px)';

function readStoredOpenState() {
  if (typeof window === 'undefined') return false;
  if (!window.matchMedia(CONTEXTUAL_AICHAT_MEDIA_QUERY).matches) return false;
  try {
    return window.sessionStorage.getItem(CONTEXTUAL_AICHAT_OPEN_STORAGE_KEY) === 'true';
  } catch {
    return false;
  }
}

function storeOpenState(isOpen: boolean) {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(CONTEXTUAL_AICHAT_OPEN_STORAGE_KEY, isOpen ? 'true' : 'false');
  } catch {
    // Session storage can be unavailable in restricted browser contexts.
  }
}

interface ContextualAIChatRegisteredGroup {
  items: AIChatContextItem[];
  priority: number;
  visibility: AIChatContextRegistrationVisibility;
  order: number;
}

interface ContextualAIChatState {
  isAvailable: boolean;
  isOpen: boolean;
  items: AIChatContextItem[];
  open: () => void;
  close: () => void;
  setOpen: (open: boolean) => void;
  registerItems: (
    items: AIChatContextItem[],
    options?: AIChatContextRegistrationOptions
  ) => () => void;
}

const ContextualAIChatContext = createContext<ContextualAIChatState | null>(null);

const CONTEXT_VISIBILITY_RANK: Record<AIChatContextRegistrationVisibility, number> = {
  current: 3,
  selected: 3,
  visible: 2,
  background: 0,
};

function normalizeCapabilities(
  capabilities: AIChatContextItem['capabilities']
): AIChatCapabilityDescriptor[] | undefined {
  const normalized: AIChatCapabilityDescriptor[] = [];
  (capabilities ?? []).forEach(capability => {
    const id = capability.id.trim();
    if (!id) return;

    normalized.push({
      ...capability,
      id,
      title: capability.title?.trim() || undefined,
      description: capability.description?.trim() || undefined,
      permissions: capability.permissions?.map(permission => permission.trim()).filter(Boolean),
    });
  });

  return normalized.length > 0 ? normalized : undefined;
}

function normalizeRelations(
  relations: AIChatContextItem['relations']
): AIChatContextRelation[] | undefined {
  const normalized: AIChatContextRelation[] = [];
  (relations ?? []).forEach(relation => {
    const type = relation.type.trim();
    const resourceId = relation.resourceId.trim();
    if (!type || !resourceId) return;

    normalized.push({
      ...relation,
      type,
      resourceId,
      title: relation.title?.trim() || undefined,
    });
  });

  return normalized.length > 0 ? normalized : undefined;
}

function normalizeContextItems(items: AIChatContextItem[]): AIChatContextItem[] {
  const seen = new Set<string>();
  const normalized: AIChatContextItem[] = [];
  items.forEach(item => {
    const resourceId = item.id.trim();
    const id = `${item.type}:${resourceId}`.trim();
    const title = item.title.trim();
    if (!resourceId || !title || seen.has(id)) return;
    seen.add(id);
    normalized.push({
      ...item,
      id: resourceId,
      title,
      subtitle: item.subtitle?.trim() || undefined,
      description: item.description?.trim() || undefined,
      source: item.source?.trim() || undefined,
      permissions: item.permissions?.map(permission => permission.trim()).filter(Boolean),
      relations: normalizeRelations(item.relations),
      capabilities: normalizeCapabilities(item.capabilities),
    });
  });
  return normalized;
}

function normalizeRegistrationPriority(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0;
  return value;
}

function normalizeRegistrationVisibility(
  value: AIChatContextRegistrationVisibility | undefined
): AIChatContextRegistrationVisibility {
  return value ?? 'visible';
}

function visibleGroups(groups: Record<string, ContextualAIChatRegisteredGroup>) {
  return Object.values(groups)
    .filter(group => group.visibility !== 'background')
    .sort((left, right) => {
      const visibilityDelta =
        CONTEXT_VISIBILITY_RANK[right.visibility] - CONTEXT_VISIBILITY_RANK[left.visibility];
      if (visibilityDelta !== 0) return visibilityDelta;
      if (right.priority !== left.priority) return right.priority - left.priority;
      return left.order - right.order;
    });
}

export function ContextualAIChatProvider({
  children,
  enabled = true,
}: {
  children: ReactNode;
  enabled?: boolean;
}) {
  const [isOpen, setOpen] = useState(readStoredOpenState);
  const [groups, setGroups] = useState<Record<string, ContextualAIChatRegisteredGroup>>({});

  useEffect(() => {
    if (!enabled && isOpen) {
      setOpen(false);
      return;
    }
    storeOpenState(enabled && isOpen);
  }, [enabled, isOpen]);

  useEffect(() => {
    const mediaQuery = window.matchMedia(CONTEXTUAL_AICHAT_MEDIA_QUERY);
    const handleChange = () => {
      if (!mediaQuery.matches) setOpen(false);
    };

    handleChange();
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  const registerItems = useCallback(
    (items: AIChatContextItem[], options?: AIChatContextRegistrationOptions) => {
      const baseScopeId = options?.scopeId?.trim() || crypto.randomUUID();
      const scopeId =
        options?.replace === false ? `${baseScopeId}:${crypto.randomUUID()}` : baseScopeId;
      const normalized = normalizeContextItems(items);
      setGroups(current => {
        if (normalized.length === 0) {
          const { [scopeId]: _removed, ...next } = current;
          return next;
        }
        return {
          ...current,
          [scopeId]: {
            items: normalized,
            priority: normalizeRegistrationPriority(options?.priority),
            visibility: normalizeRegistrationVisibility(options?.visibility),
            order: current[scopeId]?.order ?? Object.keys(current).length,
          },
        };
      });

      return () => {
        setGroups(current => {
          const { [scopeId]: _removed, ...next } = current;
          return next;
        });
      };
    },
    []
  );

  const items = useMemo(
    () => normalizeContextItems(visibleGroups(groups).flatMap(group => group.items)),
    [groups]
  );

  const setAvailableOpen = useCallback(
    (open: boolean) => {
      setOpen(enabled && open);
    },
    [enabled]
  );

  const value = useMemo<ContextualAIChatState>(
    () => ({
      isAvailable: enabled,
      isOpen: enabled && isOpen,
      items,
      open: () => setAvailableOpen(true),
      close: () => setOpen(false),
      setOpen: setAvailableOpen,
      registerItems,
    }),
    [enabled, isOpen, items, registerItems, setAvailableOpen]
  );

  return (
    <ContextualAIChatContext.Provider value={value}>{children}</ContextualAIChatContext.Provider>
  );
}

export function useContextualAIChat() {
  const context = useContext(ContextualAIChatContext);
  if (!context) {
    throw new Error('useContextualAIChat must be used within ContextualAIChatProvider');
  }
  return context;
}

export function useAIChatContextRegistration(
  items: AIChatContextItem[],
  options?: AIChatContextRegistrationOptions
) {
  const generatedScopeId = useId();
  const { registerItems } = useContextualAIChat();
  const scopeId = options?.scopeId ?? generatedScopeId;
  const replace = options?.replace;
  const priority = options?.priority;
  const visibility = options?.visibility;

  useEffect(() => {
    return registerItems(items, { scopeId, replace, priority, visibility });
  }, [items, priority, registerItems, replace, scopeId, visibility]);
}
