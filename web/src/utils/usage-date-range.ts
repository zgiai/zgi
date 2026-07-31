export function isCustomDateRangeValid(startDate: string, endDate: string): boolean {
  return Boolean(startDate && endDate && startDate <= endDate);
}
