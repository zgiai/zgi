export const GRAPH_COLOR_PALETTE = [
  { fill: '#E6F4FF', stroke: '#1677FF' }, // Blue
  { fill: '#F6FFED', stroke: '#52C41A' }, // Green
  { fill: '#FFF7E6', stroke: '#FFA940' }, // Orange
  { fill: '#FFF1F0', stroke: '#FF4D4F' }, // Red
  { fill: '#F9F0FF', stroke: '#722ED1' }, // Purple
  { fill: '#F0F5FF', stroke: '#2F54EB' }, // Indigo
  { fill: '#E6FFFB', stroke: '#13C2C2' }, // Teal
  { fill: '#FEFFE6', stroke: '#FADB14' }, // Yellow
  { fill: '#FFF0F6', stroke: '#EB2F96' }, // Magenta
  { fill: '#FCFFE6', stroke: '#A0D911' }, // Lime
];

export const DEFAULT_GRAPH_CONFIG = {
  layout: {
    type: 'force',
    preventOverlap: true,
    nodeSpacing: 80,
    linkDistance: 280,
    nodeStrength: -1000,
    edgeStrength: 0.2,
    alphaDecay: 0.01,
    velocityDecay: 0.2,
  },
  defaultNode: {
    type: 'circle',
    labelCfg: {
      position: 'bottom',
      offset: 10,
      style: {
        fill: 'hsl(var(--foreground))',
        fontSize: 12,
        fontWeight: 500,
      },
    },
  },
  defaultEdge: {
    style: {
      stroke: 'hsl(var(--border))',
      lineWidth: 1,
      opacity: 0.6,
    },
    labelCfg: {
      autoRotate: true,
      style: {
        fill: 'hsl(var(--muted-foreground))',
        fontSize: 10,
        fontWeight: 400,
        background: {
          padding: [2, 4, 2, 4],
          radius: 2,
        },
      },
    },
  },
};
