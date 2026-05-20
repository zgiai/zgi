/**
 * Compute a human-friendly slider step given a range.
 * The step scales with the span length to avoid overly fine or coarse steps.
 */
export function computeSliderStep(
  min: number,
  max: number,
  precision: number | null = null
): number {
  const span = Math.abs((max ?? 0) - (min ?? 0));
  if (!isFinite(span) || span <= 0) return 1;

  // Very small range – fallback to span/100 with lower bound 0.01
  if (span <= 1) {
    const raw = span / 100;
    const base = Math.max(0.01, raw);
    return roundToPrecision(base, precision ?? 2);
  }

  // Determine order of magnitude and pick a step that results in ~100 marks
  const order = Math.floor(Math.log10(span));
  const base = Math.pow(10, order - 2); // 1% of the leading magnitude
  // If the leading digit is large, increase step to reduce marks
  const leading = Math.floor(span / Math.pow(10, order));
  const step = leading >= 5 ? base * 10 : base;
  const finalStep = precision != null ? roundToPrecision(step, precision) : step;
  return finalStep > 0 ? finalStep : 1;
}

function roundToPrecision(value: number, precision: number): number {
  if (precision <= 0) return Math.round(value);
  const factor = Math.pow(10, precision);
  return Math.round(value * factor) / factor;
}

/**
 * Compute an initial numeric value based on optional default and bounds.
 * Designed for parameter controls: prefer default; otherwise pick midpoint.
 */
export function computeInitialNumber({
  defaultValue,
  min,
  max,
  isInt,
}: {
  defaultValue?: number | null;
  min?: number | null;
  max?: number | null;
  isInt: boolean;
}): number {
  const isInteger = Boolean(isInt);
  if (typeof defaultValue === 'number' && Number.isFinite(defaultValue)) {
    return isInteger ? Math.round(defaultValue) : defaultValue;
  }
  const minVal = typeof min === 'number' ? min : null;
  const maxVal = typeof max === 'number' ? max : null;
  if (minVal !== null && maxVal !== null) {
    const mid = minVal + (maxVal - minVal) / 2;
    const v = isInteger ? Math.round(mid) : mid;
    return Math.min(maxVal, Math.max(minVal, v));
  }
  if (minVal !== null) return isInteger ? Math.round(minVal) : minVal;
  if (maxVal !== null) return isInteger ? Math.round(maxVal) : maxVal;
  return isInteger ? 1 : 0.5;
}
