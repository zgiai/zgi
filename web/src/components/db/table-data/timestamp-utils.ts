type TimestampInput = unknown;

const WALL_TIME_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2})(?::(\d{2}))?)?$/;

function pad2(value: number): string {
  return String(value).padStart(2, '0');
}

function formatDateLocal(date: Date): string {
  if (Number.isNaN(date.getTime())) return '';
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`;
}

function normalizeWallTimeString(value: string): string {
  const trimmed = value.trim();
  const match = WALL_TIME_PATTERN.exec(trimmed);
  if (!match) return '';

  const [, year, month, day, hour = '00', minute = '00', second = '00'] = match;
  const parsed = new Date(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second)
  );

  if (
    parsed.getFullYear() !== Number(year) ||
    parsed.getMonth() !== Number(month) - 1 ||
    parsed.getDate() !== Number(day) ||
    parsed.getHours() !== Number(hour) ||
    parsed.getMinutes() !== Number(minute) ||
    parsed.getSeconds() !== Number(second)
  ) {
    return '';
  }

  return `${year}-${month}-${day} ${hour}:${minute}:${second}`;
}

function normalizeTimestamp(value: TimestampInput): string {
  if (value === null || value === undefined) return '';

  if (value instanceof Date) {
    return formatDateLocal(value);
  }

  if (typeof value === 'number') {
    const milliseconds = value < 1e12 ? value * 1000 : value;
    return formatDateLocal(new Date(milliseconds));
  }

  if (typeof value !== 'string') return '';

  const trimmed = value.trim();
  if (!trimmed) return '';

  const wallTime = normalizeWallTimeString(trimmed);
  if (wallTime) return wallTime;

  const numericValue = Number(trimmed);
  if (Number.isFinite(numericValue)) {
    const milliseconds = numericValue < 1e12 ? numericValue * 1000 : numericValue;
    return formatDateLocal(new Date(milliseconds));
  }

  const parsed = new Date(trimmed);
  return formatDateLocal(parsed);
}

export function formatTimestampWallTime(value: TimestampInput): string {
  return normalizeTimestamp(value);
}

export function formatTimestampLocalInput(value: TimestampInput): string {
  const normalized = normalizeTimestamp(value);
  if (!normalized) return '';
  return normalized.slice(0, 16).replace(' ', 'T');
}

export function datetimeLocalToWallTime(value: string): string {
  const normalized = normalizeWallTimeString(value);
  return normalized || value.trim().replace('T', ' ');
}

export function isTimestampValueValid(value: TimestampInput): boolean {
  return normalizeTimestamp(value) !== '';
}
