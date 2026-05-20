'use client';

import React, { createContext, useContext, useState, useCallback, useEffect, useMemo } from 'react';

// Panel position type for organizing stacks
type PanelPosition =
  | 'top-left'
  | 'top-right'
  | 'bottom-left'
  | 'bottom-right'
  | 'top-center'
  | 'bottom-center';

// Panel configuration for registration and offset calculation
interface PanelConfig {
  id: string;
  position: PanelPosition;
  order: number; // Lower number = higher priority in stack (appears first)
  visible: boolean;
  width: number;
  gap?: number; // Custom spacing between panels (default 8px)
}

// Calculated panel offset for applying to Panel style
interface PanelOffset {
  left: number;
  right: number;
  top: number;
  bottom: number;
}

// Context value for the provider
interface PanelStackContextValue {
  registerPanel: (config: PanelConfig) => void;
  unregisterPanel: (id: string) => void;
  updatePanel: (id: string, updates: Partial<PanelConfig>) => void;
  getPanelOffset: (id: string) => PanelOffset;
}

// Create context with default empty functions
const PanelStackContext = createContext<PanelStackContextValue>({
  registerPanel: () => {},
  unregisterPanel: () => {},
  updatePanel: () => {},
  getPanelOffset: () => ({ left: 0, right: 0, top: 0, bottom: 0 }),
});

/**
 * PanelStackProvider - Manages horizontal/vertical stacking of @xyflow/react Panel components
 * Calculates offsets to prevent overlapping when multiple panels share the same position
 */
export const PanelStackProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [panels, setPanels] = useState<Map<string, PanelConfig>>(new Map());

  // Register a new panel with the stack manager
  const registerPanel = useCallback((config: PanelConfig) => {
    setPanels(prev => {
      const updated = new Map(prev);
      const existing = updated.get(config.id);
      if (existing) {
        // If already registered, just update if changed
        const gap = config.gap ?? 8;
        if (
          existing.position === config.position &&
          existing.order === config.order &&
          existing.visible === config.visible &&
          existing.width === config.width &&
          (existing.gap ?? 8) === gap
        ) {
          return prev; // no change
        }
      }
      updated.set(config.id, { ...config, gap: config.gap ?? 8 });
      return updated;
    });
  }, []);

  // Remove panel from stack manager
  const unregisterPanel = useCallback((id: string) => {
    setPanels(prev => {
      if (!prev.has(id)) return prev;
      const updated = new Map(prev);
      updated.delete(id);
      return updated;
    });
  }, []);

  // Update existing panel configuration
  const updatePanel = useCallback((id: string, updates: Partial<PanelConfig>) => {
    setPanels(prev => {
      const existing = prev.get(id);
      if (!existing) return prev;
      const next: PanelConfig = {
        ...existing,
        ...updates,
        gap: updates.gap ?? existing.gap ?? 8,
      };
      const unchanged =
        existing.position === next.position &&
        existing.order === next.order &&
        existing.visible === next.visible &&
        existing.width === next.width &&
        (existing.gap ?? 8) === (next.gap ?? 8);
      if (unchanged) return prev; // avoid unnecessary state updates
      const updated = new Map(prev);
      updated.set(id, next);
      return updated;
    });
  }, []);

  // Calculate horizontal offset for a panel based on its position and order
  const getPanelOffset = useCallback(
    (id: string): PanelOffset => {
      const panel = panels.get(id);
      if (!panel) return { left: 0, right: 0, top: 0, bottom: 0 };

      // Get all visible panels in the same position, sorted by order
      const samePositionPanels = Array.from(panels.values())
        .filter(p => p.position === panel.position && p.visible && p.id !== id)
        .sort((a, b) => a.order - b.order);

      let leftOffset = 0;
      let rightOffset = 0;

      // Calculate horizontal offset based on position
      if (panel.position.includes('right')) {
        // For right-positioned panels, stack horizontally from right to left
        // Earlier panels (lower order) appear more to the right
        for (const p of samePositionPanels) {
          if (p.order < panel.order) {
            rightOffset += p.width + (p.gap ?? 8);
          }
        }
      } else if (panel.position.includes('left')) {
        // For left-positioned panels, stack horizontally from left to right
        // Earlier panels (lower order) appear more to the left
        for (const p of samePositionPanels) {
          if (p.order < panel.order) {
            leftOffset += p.width + (p.gap ?? 8);
          }
        }
      } else if (panel.position.includes('center')) {
        // For center panels, distribute evenly (simplified approach)
        const totalWidth = samePositionPanels.reduce((sum, p) => sum + p.width, 0) + panel.width;
        const gaps = samePositionPanels.length * 8; // Default gap
        leftOffset = -(totalWidth + gaps) / 2 + panel.order * (panel.width + 8);
      }

      // Reserve a fixed top space to avoid overlapping header in editor
      const topReserved = 40; // px; keep consistent with panels' height calc

      return {
        left: leftOffset,
        right: rightOffset,
        top: topReserved,
        bottom: 0,
      };
    },
    [panels]
  );

  const contextValue: PanelStackContextValue = useMemo(
    () => ({
      registerPanel,
      unregisterPanel,
      updatePanel,
      getPanelOffset,
    }),
    [registerPanel, unregisterPanel, updatePanel, getPanelOffset]
  );

  return <PanelStackContext.Provider value={contextValue}>{children}</PanelStackContext.Provider>;
};

/**
 * usePanelStackItem - Hook for individual panels to register and get offset
 * @param config Panel configuration
 * @returns Calculated offset styles for the Panel component
 */
export const usePanelStackItem = (config: PanelConfig) => {
  const { registerPanel, unregisterPanel, updatePanel, getPanelOffset } =
    useContext(PanelStackContext);

  // Register panel on mount, unregister on unmount
  useEffect(() => {
    // ensure default gap
    registerPanel({ ...config, gap: config.gap ?? 8 });
    return () => unregisterPanel(config.id);
    // Only depend on stable function refs and id
  }, [registerPanel, unregisterPanel, config.id]);

  // Update panel config when primitive fields change
  useEffect(() => {
    updatePanel(config.id, {
      position: config.position,
      order: config.order,
      visible: config.visible,
      width: config.width,
      gap: config.gap ?? 8,
    });
  }, [
    updatePanel,
    config.id,
    config.position,
    config.order,
    config.visible,
    config.width,
    config.gap,
  ]);

  // Compute offset directly to avoid extra state/effect loops
  const offset = getPanelOffset(config.id);

  // Convert offset to CSS style object for Panel
  const panelStyle: React.CSSProperties = {
    transform: `translate(${-offset.right + offset.left}px, ${offset.top - offset.bottom}px)`,
    '--workflow-panel-stack-right': `${offset.right}px`,
    '--workflow-panel-stack-left': `${offset.left}px`,
  } as React.CSSProperties;

  return {
    panelStyle,
    offset,
  };
};

// Export context for advanced usage
export { PanelStackContext };
