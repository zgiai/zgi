import React from 'react';
import type { NodeChange, OnNodesChange } from '@xyflow/react';
import type { WorkflowNode } from '../store/type';
import {
  applySingleNodeAlignment,
  type AlignmentGuideState,
} from '../store/helpers/alignment-guides';

type WorkflowOnNodesChange = OnNodesChange<WorkflowNode>;

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

      const result = applySingleNodeAlignment(
        nodesRef.current,
        changes as Array<NodeChange<WorkflowNode>>
      );
      setGuides(result.guides);
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
