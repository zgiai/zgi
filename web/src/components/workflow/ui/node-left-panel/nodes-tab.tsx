'use client';

import React from 'react';
import type { NodeType, NodeGroupKey } from '../create-node-modal/constants/node-types';
import { NodeCatalogList } from '../node-catalog';

export interface NodesTabProps {
  grouped: Array<{ key: NodeGroupKey; items: NodeType[] }>;
  labels: { group: Record<NodeGroupKey, string>; noAvailable: string };
  onAddCentered: (type: string) => void;
}

/**
 * @component NodesTab
 * @category Workflow
 * @status Stable
 * @description Left-panel workflow node catalog with drag-to-create behavior
 * @usage Use inside the workflow left node panel
 * @example
 * <NodesTab grouped={grouped} labels={labels} onAddCentered={addNode} />
 */
const NodesTab: React.FC<NodesTabProps> = ({ grouped, labels, onAddCentered }) => {
  return (
    <NodeCatalogList
      grouped={grouped}
      labels={labels}
      onSelect={onAddCentered}
      density="panel"
      interaction="drag"
      tooltipSide="right"
    />
  );
};

export default NodesTab;
