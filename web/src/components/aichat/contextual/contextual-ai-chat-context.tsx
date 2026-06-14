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
  AIChatContextRelation,
} from './types';

interface ContextualAIChatState {
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

export function ContextualAIChatProvider({ children }: { children: ReactNode }) {
  const [isOpen, setOpen] = useState(false);
  const [groups, setGroups] = useState<Record<string, AIChatContextItem[]>>({});

  const registerItems = useCallback(
    (items: AIChatContextItem[], options?: AIChatContextRegistrationOptions) => {
      const scopeId = options?.scopeId?.trim() || crypto.randomUUID();
      const normalized = normalizeContextItems(items);
      setGroups(current => {
        if (normalized.length === 0) {
          const { [scopeId]: _removed, ...next } = current;
          return next;
        }
        return {
          ...current,
          [scopeId]: normalized,
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

  const items = useMemo(() => normalizeContextItems(Object.values(groups).flat()), [groups]);

  const value = useMemo<ContextualAIChatState>(
    () => ({
      isOpen,
      items,
      open: () => setOpen(true),
      close: () => setOpen(false),
      setOpen,
      registerItems,
    }),
    [isOpen, items, registerItems]
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

  useEffect(() => {
    return registerItems(items, { scopeId, replace });
  }, [items, registerItems, replace, scopeId]);
}
