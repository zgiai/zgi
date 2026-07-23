import type { DatasetGraph, GraphNode } from '@/services/types/dataset';

const MAX_RENDER_NODES = 500;
const MAX_RENDER_EDGES = 1500;

export interface G6Node {
  id: string;
  label: string;
  baseLabel: string;
  category: string;
  weight: number;
  priority: number;
  data: GraphNode['data'];
}

export interface G6Edge {
  source: string;
  target: string;
  label: string;
}

export interface G6Data {
  nodes: G6Node[];
  edges: G6Edge[];
}

export const transformToG6Data = (data: DatasetGraph, selectedSourceIds: string[]): G6Data => {
  const activeNodes = data.nodes
    .filter(node => node.data.active_source_count > 0)
    .slice(0, MAX_RENDER_NODES)
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
    .slice(0, MAX_RENDER_EDGES)
    .map(edge => ({
      source: edge.source,
      target: edge.target,
      label: edge.label,
    }));

  return { nodes: activeNodes, edges };
};
