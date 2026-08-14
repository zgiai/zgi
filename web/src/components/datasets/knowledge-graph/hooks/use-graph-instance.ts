import { useEffect, useRef } from 'react';
import type {
  Graph as G6Graph,
  IG6GraphEvent,
  INode,
  IShape,
  ModelConfig,
  NodeConfig,
} from '@antv/g6';
import { DEFAULT_GRAPH_CONFIG } from '../constants';
import { registerAutoAdaptLabel } from '../behaviors/auto-adapt-label';
import type { G6Data, G6Node } from '../utils/data-adapter';

interface UseGraphInstanceProps {
  containerRef: React.RefObject<HTMLDivElement>;
  data: G6Data;
  onNodeClick?: (node: G6Node) => void;
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
}

interface GraphHandle {
  instance: G6Graph;
  focusNode: (nodeId: string) => void;
}

type DraggableModel = ModelConfig & {
  fx?: number | null;
  fy?: number | null;
};

export const useGraphInstance = ({
  containerRef,
  data,
  onNodeClick,
  categoryColorMap,
}: UseGraphInstanceProps) => {
  const graphRef = useRef<GraphHandle | null>(null);
  const activeGraphRef = useRef<G6Graph | null>(null);
  const dataRef = useRef(data);
  const onNodeClickRef = useRef(onNodeClick);
  const categoryColorMapRef = useRef(categoryColorMap);
  dataRef.current = data;
  onNodeClickRef.current = onNodeClick;
  categoryColorMapRef.current = categoryColorMap;
  const hasData = data.nodes.length > 0;

  useEffect(() => {
    if (!containerRef.current || !hasData) return;

    let activeGraph: G6Graph | null = null;
    let isTerminated = false;

    const init = async () => {
      // @ts-ignore
      const G6Module = await import('@antv/g6/dist/g6.min.js');
      const G6 = G6Module.default || G6Module;

      if (isTerminated || !containerRef.current) return;

      // Register behaviors
      registerAutoAdaptLabel(G6);

      const width = containerRef.current.clientWidth || 800;
      const height = containerRef.current.clientHeight || 600;

      const graph = new G6.Graph({
        container: containerRef.current,
        width,
        height,
        ...DEFAULT_GRAPH_CONFIG,
        modes: {
          default: [
            'drag-canvas',
            'zoom-canvas',
            { type: 'drag-node', enableDelegate: false },
            'custom-auto-adapt-label',
          ],
        },
      }) as G6Graph;
      activeGraph = graph;

      // Custom node styling
      graph.node((node: NodeConfig) => {
        const graphNode = node as NodeConfig & Pick<G6Node, 'category' | 'weight'>;
        const colors = categoryColorMapRef.current[graphNode.category] || {
          fill: '#F0F5FF',
          stroke: '#2F54EB',
          text: '#1D39C4',
        };
        const size = Math.min(Math.max(graphNode.weight * 0.5 + 26, 28), 80);

        return {
          size,
          labelCfg: {
            style: {
              fill: colors.text,
              fontWeight: 600,
            },
          },
          style: {
            fill: colors.fill,
            stroke: colors.stroke,
            lineWidth: 2,
          },
        };
      });

      graph.data(dataRef.current);
      graph.render();
      activeGraphRef.current = graph;

      // Event Listeners
      graph.on('node:click', (e: IG6GraphEvent) => {
        if (!e.item) return;
        const clickedItem = e.item as INode;
        const model = clickedItem.getModel() as G6Node;
        graph.set('selectedLabelNodeId', clickedItem.getID());
        onNodeClickRef.current?.(model);

        // Visual focus logic
        const neighbors = clickedItem.getNeighbors();
        const neighborIds = new Set(neighbors.map(neighbor => neighbor.getID()));
        const clickedNodeId = clickedItem.getID();

        graph.getNodes().forEach(node => {
          const id = node.getID();
          const isFocus = id === clickedNodeId || neighborIds.has(id);
          const group = node.getContainer();
          const label = group.find((element: IShape) => element.get('name') === 'text-shape');
          if (label) {
            label.attr('opacity', isFocus ? 1 : 0.1);
            if (id === clickedNodeId) {
              label.show();
            }
          }
          graph.updateItem(node, {
            style: { opacity: isFocus ? 1 : 0.2 },
          });
        });

        graph.getEdges().forEach(edge => {
          const sourceId = edge.getSource().getID();
          const targetId = edge.getTarget().getID();
          const isRelated = sourceId === clickedNodeId || targetId === clickedNodeId;
          graph.updateItem(edge, {
            labelCfg: { style: { opacity: isRelated ? 1 : 0 } },
            style: { opacity: isRelated ? 1 : 0.1 },
          });
        });
      });

      const labelVisibilityBeforeHover = new Map<string, boolean>();

      graph.on('node:mouseenter', (e: IG6GraphEvent) => {
        if (!e.item) return;

        const nodeId = e.item.getID();
        const group = e.item.getContainer();
        const label = group.find((ele: IShape) => ele.get('name') === 'text-shape');
        if (!label) return;

        labelVisibilityBeforeHover.set(nodeId, label.get('visible') !== false);
        label.show();
      });

      graph.on('node:mouseleave', (e: IG6GraphEvent) => {
        if (!e.item) return;

        const nodeId = e.item.getID();
        const wasVisible = labelVisibilityBeforeHover.get(nodeId);
        labelVisibilityBeforeHover.delete(nodeId);

        if (wasVisible !== false || graph.get('selectedLabelNodeId') === nodeId) return;

        const group = e.item.getContainer();
        const label = group.find((ele: IShape) => ele.get('name') === 'text-shape');
        label?.hide();
      });

      graph.on('canvas:click', () => {
        graph.set('selectedLabelNodeId', undefined);
        graph.getNodes().forEach(node => {
          const group = node.getContainer();
          const label = group.find((element: IShape) => element.get('name') === 'text-shape');
          if (label) {
            label.attr('opacity', 1);
          }
          graph.updateItem(node, {
            style: { opacity: 1 },
          });
        });
        graph.getEdges().forEach(edge => {
          graph.updateItem(edge, {
            style: { opacity: 0.6 },
            labelCfg: { style: { opacity: 1 } },
          });
        });
        graph.emit('afterlayout');
      });

      // Drag behaviors
      graph.on('node:dragstart', (e: IG6GraphEvent) => {
        if (!e.item) return;
        graph.layout();
        const model = e.item.getModel() as DraggableModel;
        model.fx = e.x;
        model.fy = e.y;
      });

      graph.on('node:drag', (e: IG6GraphEvent) => {
        if (!e.item) return;
        const model = e.item.getModel() as DraggableModel;
        model.fx = e.x;
        model.fy = e.y;
        const layoutInstance = graph.get('layoutController').layoutMethods[0];
        if (layoutInstance) {
          layoutInstance.alpha = 0.3;
          layoutInstance.execute();
        }
      });

      graph.on('node:dragend', (e: IG6GraphEvent) => {
        if (!e.item) return;
        const model = e.item.getModel() as DraggableModel;
        model.fx = null;
        model.fy = null;
      });

      const focusNode = (nodeId: string) => {
        if (graph.destroyed) return;
        const item = graph.findById(nodeId);
        if (item) {
          graph.focusItem(item, true);
          graph.emit('node:click', { item });
        }
      };

      if (!isTerminated) {
        graphRef.current = {
          instance: graph,
          focusNode,
        };
      } else {
        graph.destroy();
      }
    };

    init();

    const handleResize = () => {
      if (containerRef.current && activeGraph && !activeGraph.destroyed) {
        activeGraph.changeSize(containerRef.current.clientWidth, containerRef.current.clientHeight);
      }
    };

    window.addEventListener('resize', handleResize);

    return () => {
      isTerminated = true;
      window.removeEventListener('resize', handleResize);
      if (activeGraph) {
        activeGraph.destroy();
      }
      if (activeGraphRef.current === activeGraph) {
        activeGraphRef.current = null;
      }
      graphRef.current = null;
    };
  }, [containerRef, hasData]);

  useEffect(() => {
    const activeGraph = activeGraphRef.current;
    if (!activeGraph || activeGraph.destroyed) return;

    const positions = new Map(
      activeGraph.getNodes().map(node => {
        const model = node.getModel();
        return [node.getID(), { x: model.x, y: model.y }];
      })
    );
    const nextData = {
      ...data,
      nodes: data.nodes.map(node => {
        const position = positions.get(node.id) as
          | { x: number | undefined; y: number | undefined }
          | undefined;
        return position ? { ...node, ...position } : node;
      }),
    };
    activeGraph.changeData(nextData);
  }, [data]);

  return graphRef;
};
