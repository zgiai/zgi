/**
 * Canonical billing units (frozen contract: the v1 unit registry).
 *
 * 1 USD = 1,000 points = 1,000,000 milli-points.
 *
 * The backend wire and ledger amount is always an integer count of
 * milli-points; the UI works in points. Every scale in the app must derive
 * from this single registry so a "credit" and a "point" can never carry
 * different magnitudes.
 */
export const POINTS_PER_USD = 1_000;
export const MILLI_POINTS_PER_POINT = 1_000;
export const MILLI_POINTS_PER_USD = POINTS_PER_USD * MILLI_POINTS_PER_POINT;
