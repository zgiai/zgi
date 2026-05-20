import React from 'react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import type { WorkflowVariableCatalogGroup } from '../../hooks';

export interface NodeTabProps {
  node: WorkflowVariableCatalogGroup;
  isActive: boolean;
  onClick: () => void;
  className?: string;
  ariaLabel?: string;
}

/**
 * NodeTab - pill-style tab for selecting an upstream node.
 */
const NodeTab: React.FC<NodeTabProps> = ({ node, isActive, onClick, className, ariaLabel }) => {
  const displayTitle = node.sourceTitle || `${node.sourceNodeType} Node`;

  return (
    <Button
      variant={isActive ? 'default' : 'ghost'}
      size="sm"
      onClick={onClick}
      aria-label={ariaLabel}
      className={cn(
        'h-7 px-3 text-xs font-medium rounded-md',
        'transition-colors duration-200',
        isActive && 'bg-success/20 text-success hover:bg-success/20',
        className
      )}
    >
      {displayTitle}
    </Button>
  );
};

export default React.memo(NodeTab);
