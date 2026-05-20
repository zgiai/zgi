import type { SseMessage } from './types';

export interface SseRawEvent {
  event: string | null;
  id: string | null;
  retry: number | null;
  raw: string;
}

export interface SseParsedEvent<TOut> extends SseMessage<TOut> {
  incompleteJson: boolean;
  parsedJson: boolean;
}

export interface SseParserFinishResult {
  events: SseRawEvent[];
  hadPendingEvent: boolean;
}

/**
 * @util SseParser
 * @description Incrementally parses Server-Sent Events framing from decoded text chunks.
 */
export class SseParser {
  private buffer = '';
  private eventName: string | null = null;
  private lastEventId: string | null = null;
  private retryMs: number | null = null;
  private dataBuffer: string[] = [];

  push(chunk: string): SseRawEvent[] {
    if (!chunk) {
      return [];
    }

    const events: SseRawEvent[] = [];
    this.buffer += chunk;

    let lineEnd: number;
    while ((lineEnd = this.buffer.indexOf('\n')) !== -1) {
      const line = this.buffer.slice(0, lineEnd);
      this.buffer = this.buffer.slice(lineEnd + 1);
      const event = this.processLine(line);
      if (event) {
        events.push(event);
      }
    }

    return events;
  }

  finish(): SseParserFinishResult {
    const events: SseRawEvent[] = [];

    if (this.buffer.length > 0) {
      const event = this.processLine(this.buffer);
      this.buffer = '';
      if (event) {
        events.push(event);
      }
    }

    const hadPendingEvent = this.hasPendingEvent();
    const event = this.dispatchEvent();
    if (event) {
      events.push(event);
    }

    return { events, hadPendingEvent };
  }

  private processLine(lineRaw: string): SseRawEvent | null {
    const line = lineRaw.replace(/\r$/, '');
    if (line === '') {
      return this.dispatchEvent();
    }

    if (line.startsWith(':')) {
      return null;
    }

    const colonIndex = line.indexOf(':');
    const field = colonIndex === -1 ? line : line.slice(0, colonIndex);
    let value = colonIndex === -1 ? '' : line.slice(colonIndex + 1);
    if (value.startsWith(' ')) {
      value = value.slice(1);
    }

    switch (field) {
      case 'event':
        this.eventName = value;
        break;
      case 'data':
        this.dataBuffer.push(value);
        break;
      case 'id':
        this.lastEventId = value;
        break;
      case 'retry': {
        const n = Number(value);
        this.retryMs = Number.isFinite(n) ? n : this.retryMs;
        break;
      }
    }

    return null;
  }

  private dispatchEvent(): SseRawEvent | null {
    if (this.dataBuffer.length === 0) {
      this.eventName = null;
      this.retryMs = null;
      return null;
    }

    const event: SseRawEvent = {
      event: this.eventName,
      id: this.lastEventId,
      retry: this.retryMs,
      raw: this.dataBuffer.join('\n'),
    };

    this.eventName = null;
    this.retryMs = null;
    this.dataBuffer = [];
    return event;
  }

  private hasPendingEvent(): boolean {
    return (
      this.buffer.length > 0 ||
      this.eventName !== null ||
      this.retryMs !== null ||
      this.dataBuffer.length > 0
    );
  }
}

export function parseSseRawEvent<TOut>(event: SseRawEvent): SseParsedEvent<TOut> {
  let parsed: TOut | null = null;
  let parsedJson = false;
  let incompleteJson = false;

  try {
    parsed = JSON.parse(event.raw) as TOut;
    parsedJson = true;
  } catch (error) {
    incompleteJson = isIncompleteJsonParseError(error, event.raw);
  }

  return {
    event: event.event,
    data: parsedJson ? (parsed as TOut) : (event.raw as unknown as TOut),
    id: event.id,
    raw: event.raw,
    retry: event.retry,
    incompleteJson,
    parsedJson,
  };
}

export function isIncompleteJsonParseError(error: unknown, raw: string): boolean {
  if (!(error instanceof SyntaxError)) {
    return false;
  }

  const trimmed = raw.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[') && !trimmed.startsWith('"')) {
    return false;
  }

  const message = error.message.toLowerCase();
  return (
    message.includes('unexpected end of json input') ||
    message.includes('unterminated string') ||
    message.includes('unterminated string literal') ||
    message.includes('end of data') ||
    hasUnclosedJsonStructure(trimmed)
  );
}

function hasUnclosedJsonStructure(raw: string): boolean {
  let depth = 0;
  let inString = false;
  let escaped = false;

  for (const char of raw) {
    if (escaped) {
      escaped = false;
      continue;
    }

    if (char === '\\' && inString) {
      escaped = true;
      continue;
    }

    if (char === '"') {
      inString = !inString;
      continue;
    }

    if (inString) {
      continue;
    }

    if (char === '{' || char === '[') {
      depth += 1;
    } else if (char === '}' || char === ']') {
      depth = Math.max(0, depth - 1);
    }
  }

  return inString || depth > 0;
}
