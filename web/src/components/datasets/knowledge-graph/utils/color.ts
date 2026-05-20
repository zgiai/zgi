/**
 * Generates a visually distinct color for a given index using the Golden Ratio for hue distribution.
 * This ensures that colors are spread out as much as possible on the color wheel.
 */
export const generateCategoryColor = (index: number) => {
  const goldenRatioConjugate = 0.618033988749895;
  let h = (index * goldenRatioConjugate) % 1;
  const hue = h * 360;

  // We use fixed saturation and lightness for a consistent "premium" look
  // Pastels for fill, more vibrant for stroke/text
  return {
    fill: `hsl(${hue}, 70%, 94%)`,
    stroke: `hsl(${hue}, 80%, 45%)`,
    text: `hsl(${hue}, 80%, 35%)`, // Slightly darker for readability
  };
};

/**
 * Generates a map of category IDs to their respective colors.
 */
export const getCategoryColorMap = (categories: { id: string }[]) => {
  const map: Record<string, { fill: string; stroke: string; text: string }> = {};
  categories.forEach((cat, index) => {
    map[cat.id] = generateCategoryColor(index);
  });
  return map;
};
