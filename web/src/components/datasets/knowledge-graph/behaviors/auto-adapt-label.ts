export const registerAutoAdaptLabel = (G6: any) => {
  if (G6.hasRegisteredBehavior?.('custom-auto-adapt-label')) return;

  G6.registerBehavior('custom-auto-adapt-label', {
    getEvents() {
      return {
        afterlayout: 'handleAutoAdapt',
        viewportchange: 'handleAutoAdapt',
        'node:dragend': 'handleAutoAdapt',
        'canvas:dragend': 'handleAutoAdapt',
        'node:mouseenter': 'handleAutoAdapt',
        'node:mouseleave': 'handleAutoAdapt',
        'node:statechange': 'handleAutoAdapt',
        afterupdate: 'handleAutoAdapt',
      };
    },
    handleAutoAdapt() {
      // Use requestAnimationFrame to ensure we run AFTER G6's internal style resets
      // which often happen synchronously during state changes.
      requestAnimationFrame(() => {
        if (this.destroyed) return;
        const graph = this.graph;
        if (!graph || graph.destroyed) return;

        const zoom = graph.getZoom();
        const nodes = graph.getNodes();

        // Sort nodes by priority (highest first)
        const sortedNodes = [...nodes].sort((a, b) => {
          const priorityA = a.getModel().priority || 0;
          const priorityB = b.getModel().priority || 0;
          return priorityB - priorityA;
        });

        const occupiedBoxes: any[] = [];

        sortedNodes.forEach(node => {
          const group = node.getContainer();
          const label = group.find((ele: any) => ele.get('name') === 'text-shape');
          if (!label) return;

          // Ensure label size is constant 12px visually
          label.attr('fontSize', 12 / zoom);

          // Get actual bounding box in canvas coordinates
          const bbox = label.getCanvasBBox();

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
