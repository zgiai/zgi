import { useEffect, useRef } from 'react';
import { DEFAULT_GRAPH_CONFIG } from '../constants';
import { registerAutoAdaptLabel } from '../behaviors/auto-adapt-label';
import type { G6Data } from '../utils/data-adapter';

interface UseGraphInstanceProps {
  containerRef: React.RefObject<HTMLDivElement>;
  data: G6Data;
  onNodeClick?: (node: any) => void;
  categoryColorMap: Record<string, { fill: string; stroke: string; text: string }>;
}

export const useGraphInstance = ({
  containerRef,
  data,
  onNodeClick,
  categoryColorMap,
}: UseGraphInstanceProps) => {
  const graphRef = useRef<any>(null);

  useEffect(() => {
    if (!containerRef.current || !data.nodes.length) return;

    let activeGraph: any = null;
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

      activeGraph = new G6.Graph({
        container: containerRef.current,
        width,
        height,
        ...DEFAULT_GRAPH_CONFIG,
        modes: {
          default: [
            'drag-canvas',
            'zoom-canvas',
            { type: 'drag-node', enableDelegate: false },
            'activate-relations',
            'custom-auto-adapt-label',
          ],
        },
      });

      // Custom node styling
      activeGraph.node((node: any) => {
        const colors = categoryColorMap[node.category] || {
          fill: '#F0F5FF',
          stroke: '#2F54EB',
          text: '#1D39C4',
        };
        const size = Math.min(Math.max(node.weight * 0.5 + 26, 28), 80);

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

      activeGraph.data(data);
      activeGraph.render();

      // Event Listeners
      activeGraph.on('node:click', (e: any) => {
        const clickedItem = e.item;
        const model = clickedItem.getModel();
        onNodeClick?.(model);

        // Visual focus logic
        const neighbors = clickedItem.getNeighbors();
        const neighborIds = new Set(neighbors.map((n: any) => n.getID()));
        const clickedNodeId = clickedItem.getID();

        activeGraph.getNodes().forEach((node: any) => {
          const id = node.getID();
          const isFocus = id === clickedNodeId || neighborIds.has(id);
          const group = node.getContainer();
          const label = group.find((ele: any) => ele.get('name') === 'text-shape');
          if (label) {
            label.attr('opacity', isFocus ? 1 : 0.1);
          }
          activeGraph.updateItem(node, {
            style: { opacity: isFocus ? 1 : 0.2 },
          });
        });

        activeGraph.getEdges().forEach((edge: any) => {
          const sourceId = edge.getSource().getID();
          const targetId = edge.getTarget().getID();
          const isRelated = sourceId === clickedNodeId || targetId === clickedNodeId;
          activeGraph.updateItem(edge, {
            labelCfg: { style: { opacity: isRelated ? 1 : 0 } },
            style: { opacity: isRelated ? 1 : 0.1 },
          });
        });
      });

      activeGraph.on('canvas:click', () => {
        activeGraph.getNodes().forEach((node: any) => {
          const group = node.getContainer();
          const label = group.find((ele: any) => ele.get('name') === 'text-shape');
          if (label) {
            label.attr('opacity', 1);
          }
          activeGraph.updateItem(node, {
            style: { opacity: 1 },
          });
        });
        activeGraph.getEdges().forEach((edge: any) => {
          activeGraph.updateItem(edge, {
            style: { opacity: 0.6 },
            labelCfg: { style: { opacity: 1 } },
          });
        });
        activeGraph.emit('afterlayout');
      });

      // Drag behaviors
      activeGraph.on('node:dragstart', (e: any) => {
        activeGraph.layout();
        const model = e.item.get('model');
        model.fx = e.x;
        model.fy = e.y;
      });

      activeGraph.on('node:drag', (e: any) => {
        const model = e.item.get('model');
        model.fx = e.x;
        model.fy = e.y;
        const layoutInstance = activeGraph.get('layoutController').layoutMethods[0];
        if (layoutInstance) {
          layoutInstance.alpha = 0.3;
          layoutInstance.execute();
        }
      });

      activeGraph.on('node:dragend', (e: any) => {
        const model = e.item.get('model');
        model.fx = null;
        model.fy = null;
      });

      const focusNode = (nodeId: string) => {
        if (!activeGraph || activeGraph.destroyed) return;
        const item = activeGraph.findById(nodeId);
        if (item) {
          activeGraph.focusItem(item, true);
          activeGraph.emit('node:click', { item });
        }
      };

      if (!isTerminated) {
        graphRef.current = {
          instance: activeGraph,
          focusNode,
        };
      } else {
        activeGraph.destroy();
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
      graphRef.current = null;
    };
  }, [data, onNodeClick, categoryColorMap]);

  return graphRef;
};
