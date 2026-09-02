export function formatTokenCount(value?: number | string | null, locale?: string): string {
  if (value === undefined || value === null || value === '') return '-';

  const count = Number(value);
  if (!Number.isFinite(count)) return '-';

  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 0,
    minimumFractionDigits: 0,
    useGrouping: true,
  }).format(Math.trunc(count));
}
