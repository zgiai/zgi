import type { DatasetGraph, GraphNode } from '@/services/types/dataset';
import type { EdgeConfig, NodeConfig } from '@antv/g6';

export type G6Node = GraphNode &
  NodeConfig & {
    label: string;
    baseLabel: string;
    weight: number;
    priority: number;
    x?: number;
    y?: number;
  };

export type G6Edge = EdgeConfig & {
  source: string;
  target: string;
  label: string;
};

export interface G6Data {
  nodes: G6Node[];
  edges: G6Edge[];
}

export const transformToG6Data = (data: DatasetGraph, selectedSourceIds: string[]): G6Data => {
  const activeNodes = data.nodes
    .filter(node => node.data.active_source_count > 0)
    .map((node, index) => {
      const activeSources =
        node.data?.sources?.filter(source => selectedSourceIds.includes(source.doc.id)) || [];
      const totalWeight = activeSources.reduce((sum, source) => sum + source.weight, 0);

      return {
        ...node,
        id: node.id,
        label: `${node.label} (${totalWeight})`,
        baseLabel: node.label,
        category: node.category,
        weight: totalWeight,
        priority: totalWeight + index * 0.001,
        data: node.data,
      };
    })
    .filter(node => node.weight > 0);

  const nodeIds = new Set(activeNodes.map(n => n.id));
  const edges = data.edges
    .filter(edge => edge.active_weight > 0 && nodeIds.has(edge.source) && nodeIds.has(edge.target))
    .map(edge => ({
      source: edge.source,
      target: edge.target,
      label: edge.label,
    }));

  return { nodes: activeNodes, edges };
};
