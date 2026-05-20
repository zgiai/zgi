import * as React from 'react';

import {
  getSidebarCollapsed,
  saveSidebarCollapsed,
  type SidebarId,
} from '@/utils/ui-local';

export function usePersistentSidebarCollapse(
  id: SidebarId,
  fallback: boolean,
  temporarilyCollapsed = false
) {
  const [isCollapsed, setIsCollapsed] = React.useState<boolean>(() =>
    getSidebarCollapsed(id, fallback)
  );
  const temporaryRestoreCollapsedRef = React.useRef<boolean | null>(null);
  const temporaryWasActiveRef = React.useRef(false);
  const skipNextPersistRef = React.useRef(false);

  React.useEffect(() => {
    if (skipNextPersistRef.current) {
      skipNextPersistRef.current = false;
      return;
    }
    saveSidebarCollapsed(id, isCollapsed);
  }, [id, isCollapsed]);

  React.useEffect(() => {
    if (temporarilyCollapsed && !temporaryWasActiveRef.current) {
      temporaryWasActiveRef.current = true;
      temporaryRestoreCollapsedRef.current = isCollapsed;
      if (!isCollapsed) {
        skipNextPersistRef.current = true;
        setIsCollapsed(true);
      }
      return;
    }

    if (!temporarilyCollapsed && temporaryWasActiveRef.current) {
      temporaryWasActiveRef.current = false;
      const restoreCollapsed = temporaryRestoreCollapsedRef.current;
      temporaryRestoreCollapsedRef.current = null;
      if (restoreCollapsed !== null) setIsCollapsed(restoreCollapsed);
    }
  }, [isCollapsed, temporarilyCollapsed]);

  return [isCollapsed, setIsCollapsed] as const;
}
