import type G6Module from '@antv/g6';
import type { Graph } from '@antv/g6';

interface AutoAdaptLabelState {
  adaptFrame?: number | null;
  destroyed?: boolean;
  graph?: Graph;
}

interface LabelBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

export const registerAutoAdaptLabel = (
  G6: typeof G6Module & { hasRegisteredBehavior?: (name: string) => boolean }
) => {
  if (G6.hasRegisteredBehavior?.('custom-auto-adapt-label')) return;

  G6.registerBehavior('custom-auto-adapt-label', {
    getEvents() {
      return {
        afterlayout: 'handleAutoAdapt',
        viewportchange: 'handleAutoAdapt',
        'node:dragend': 'handleAutoAdapt',
        'canvas:dragend': 'handleAutoAdapt',
        afterupdate: 'handleAutoAdapt',
      };
    },
    handleAutoAdapt(this: AutoAdaptLabelState) {
      // Collapse bursts of layout and viewport events into one deterministic pass.
      // Pointer movement must not recalculate the visibility of every label.
      if (this.adaptFrame) {
        cancelAnimationFrame(this.adaptFrame);
      }

      this.adaptFrame = requestAnimationFrame(() => {
        this.adaptFrame = null;
        if (this.destroyed) return;
        const graph = this.graph;
        if (!graph || graph.destroyed) return;

        const zoom = Math.max(graph.getZoom(), 0.01);
        const nodes = graph.getNodes();
        const selectedNodeId = graph.get('selectedLabelNodeId');

        // Keep the selected entity visible, then retain higher-priority labels.
        const sortedNodes = [...nodes].sort((a, b) => {
          const selectedA = a.getID() === selectedNodeId ? 1 : 0;
          const selectedB = b.getID() === selectedNodeId ? 1 : 0;
          if (selectedA !== selectedB) return selectedB - selectedA;

          const priorityA = Number(a.getModel().priority) || 0;
          const priorityB = Number(b.getModel().priority) || 0;
          return priorityB - priorityA;
        });

        const occupiedBoxes: LabelBox[] = [];
        const labelEntries = sortedNodes
          .map(node => {
            const group = node.getContainer();
            const label = group.find(ele => ele.get('name') === 'text-shape');
            if (!label) return null;

            // Measure every label from the same visible state. Measuring a previously
            // hidden shape can otherwise produce a stale or empty bounding box.
            label.show();
            label.attr('fontSize', 12 / zoom);

            return { label, bbox: label.getCanvasBBox() };
          })
          .filter((entry): entry is NonNullable<typeof entry> => entry !== null);

        labelEntries.forEach(entry => {
          const { label, bbox } = entry;

          // Add some padding to the bbox for better clearance
          const padding = 4;
          const boxWithPadding = {
            x: bbox.x - padding,
            y: bbox.y - padding,
            width: bbox.width + padding * 2,
            height: bbox.height + padding * 2,
          };

          // Check for overlap with already visible labels
          let isOverlapping = false;
          for (const occupied of occupiedBoxes) {
            if (
              boxWithPadding.x < occupied.x + occupied.width &&
              boxWithPadding.x + boxWithPadding.width > occupied.x &&
              boxWithPadding.y < occupied.y + occupied.height &&
              boxWithPadding.y + boxWithPadding.height > occupied.y
            ) {
              isOverlapping = true;
              break;
            }
          }

          if (isOverlapping) {
            label.hide();
          } else {
            label.show();
            occupiedBoxes.push(boxWithPadding);
          }
        });
      });
    },
  });
};
