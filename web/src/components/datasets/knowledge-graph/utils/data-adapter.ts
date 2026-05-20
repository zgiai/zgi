import type { DatasetGraph } from '@/services/types/dataset';

export interface G6Node {
  id: string;
  label: string;
  baseLabel: string;
  category: string;
  weight: number;
  priority: number;
  data: any;
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
    .map((node, index) => {
      const activeSources =
        node.data?.sources?.filter((s: any) => selectedSourceIds.includes(s.doc.id)) || [];
      const totalWeight = activeSources.reduce((sum: number, s: any) => sum + s.weight, 0);

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
    .filter(edge => nodeIds.has(edge.source) && nodeIds.has(edge.target))
    .map(edge => ({
      source: edge.source,
      target: edge.target,
      label: edge.label,
    }));

  return { nodes: activeNodes, edges };
};
