'use client';

import { memo } from 'react';

export interface CollapseControlsProps {
  expandAllText: string;
  collapseAllText: string;
  onExpandAll: () => void;
  onCollapseAll: () => void;
}

// Expand/Collapse all controls component
export const CollapseControls = memo(function CollapseControls({
  expandAllText,
  collapseAllText,
  onExpandAll,
  onCollapseAll,
}: CollapseControlsProps) {
  return (
    <div className="px-2 py-1 border-b">
      <div className="flex gap-2">
        <button
          type="button"
          className="flex-1 text-xs px-2 py-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 rounded transition-colors"
          onMouseDown={e => {
            e.preventDefault();
            e.stopPropagation();
            onExpandAll();
          }}
        >
          {expandAllText}
        </button>
        <button
          type="button"
          className="flex-1 text-xs px-2 py-1 text-muted-foreground hover:text-foreground hover:bg-accent/50 rounded transition-colors"
          onMouseDown={e => {
            e.preventDefault();
            e.stopPropagation();
            onCollapseAll();
          }}
        >
          {collapseAllText}
        </button>
      </div>
    </div>
  );
});
