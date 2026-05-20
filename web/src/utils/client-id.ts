let fallbackCounter = 0;

const BYTE_TO_HEX = Array.from({ length: 256 }, (_, index) =>
  index.toString(16).padStart(2, '0')
);

/**
 * @util generateClientId
 * @description Generate a UUID v4-shaped client id without relying on browser UUID APIs.
 */
export function generateClientId(prefix?: string): string {
  const bytes = createClientIdBytes();

  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;

  const id = formatUuidBytes(bytes);
  return prefix ? `${prefix}-${id}` : id;
}

function createClientIdBytes(): Uint8Array {
  const bytes = new Uint8Array(16);
  const cryptoRef = globalThis.crypto;

  if (typeof cryptoRef?.getRandomValues === 'function') {
    try {
      cryptoRef.getRandomValues(bytes);
      return bytes;
    } catch {
      // Fall through to the timestamp/random fallback below.
    }
  }

  fillFallbackBytes(bytes);
  return bytes;
}

function fillFallbackBytes(bytes: Uint8Array): void {
  fallbackCounter = (fallbackCounter + 1) % Number.MAX_SAFE_INTEGER;

  const timestamp = Date.now();
  for (let i = 0; i < 6; i += 1) {
    bytes[i] = Math.floor(timestamp / 2 ** ((5 - i) * 8)) & 0xff;
  }

  for (let i = 0; i < 4; i += 1) {
    bytes[6 + i] = Math.floor(fallbackCounter / 2 ** ((3 - i) * 8)) & 0xff;
  }

  for (let i = 10; i < bytes.length; i += 1) {
    bytes[i] = Math.floor(Math.random() * 256) & 0xff;
  }
}

function formatUuidBytes(bytes: Uint8Array): string {
  return [
    bytesToHex(bytes, 0, 4),
    bytesToHex(bytes, 4, 6),
    bytesToHex(bytes, 6, 8),
    bytesToHex(bytes, 8, 10),
    bytesToHex(bytes, 10, 16),
  ].join('-');
}

function bytesToHex(bytes: Uint8Array, start: number, end: number): string {
  let value = '';
  for (let i = start; i < end; i += 1) {
    value += BYTE_TO_HEX[bytes[i]];
  }
  return value;
}
