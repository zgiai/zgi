import React from 'react';
import type { NodeChange, OnNodesChange } from '@xyflow/react';
import type { WorkflowNode } from '../store/type';
import {
  applySingleNodeAlignment,
  type AlignmentGuideState,
} from '../store/helpers/alignment-guides';

type WorkflowOnNodesChange = OnNodesChange<WorkflowNode>;

function hasActiveNodeDrag(changes: Array<NodeChange<WorkflowNode>>): boolean {
  return changes.some(
    change => change.type === 'position' && 'dragging' in change && change.dragging === true
  );
}

export function useNodeAlignmentGuides({
  nodes,
  disabled,
  onNodesChange,
}: {
  nodes: WorkflowNode[];
  disabled: boolean;
  onNodesChange?: WorkflowOnNodesChange;
}) {
  const [guides, setGuides] = React.useState<AlignmentGuideState | null>(null);
  const nodesRef = React.useRef(nodes);

  React.useEffect(() => {
    nodesRef.current = nodes;
  }, [nodes]);

  const clearAlignmentGuides = React.useCallback(() => {
    setGuides(null);
  }, []);

  const onNodesChangeWithAlignment = React.useCallback<WorkflowOnNodesChange>(
    changes => {
      if (disabled || !onNodesChange) {
        clearAlignmentGuides();
        onNodesChange?.(changes);
        return;
      }

      const workflowChanges = changes as Array<NodeChange<WorkflowNode>>;
      const result = applySingleNodeAlignment(nodesRef.current, workflowChanges);
      setGuides(hasActiveNodeDrag(workflowChanges) ? result.guides : null);
      onNodesChange(result.changes);
    },
    [clearAlignmentGuides, disabled, onNodesChange]
  );

  return {
    alignmentGuides: guides,
    clearAlignmentGuides,
    onNodesChangeWithAlignment,
  };
}
