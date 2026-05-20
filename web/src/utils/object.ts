/**
 * Object utility functions
 * Common operations for object manipulation
 */

/**
 * Deep clones an object
 * @param obj - Object to clone
 */
export function deepClone<T>(obj: T): T {
  // Prefer native structuredClone when available for performance and correctness
  const sc = (globalThis as { structuredClone?: <U>(value: U) => U }).structuredClone;
  if (sc) {
    try {
      return sc(obj);
    } catch {
      // Fall back to manual deep clone for values not supported by structuredClone
    }
  }

  if (obj === null || typeof obj !== 'object') {
    return obj;
  }

  // Handle Date instance
  if (obj instanceof Date) {
    return new Date(obj.getTime()) as unknown as T;
  }

  // Handle Array
  if (Array.isArray(obj)) {
    const arr = obj as unknown[];
    return arr.map(item => deepClone(item)) as unknown as T;
  }

  // Handle plain object
  const input = obj as unknown as Record<string, unknown>;
  const output: Record<string, unknown> = {};
  for (const key in input) {
    if (Object.prototype.hasOwnProperty.call(input, key)) {
      output[key] = deepClone(input[key]);
    }
  }
  return output as unknown as T;
}

/**
 * Safely gets a nested property from an object using a path string
 * @param obj - The object to get the value from
 * @param path - Path to the property (e.g., 'user.address.city')
 * @param defaultValue - Default value if path doesn't exist
 */
export function getNestedValue<T, D = undefined>(
  obj: Record<string, unknown>,
  path: string,
  defaultValue?: D
): T | D {
  const keys = path.split('.');
  let result: unknown = obj;

  for (const key of keys) {
    if (result === undefined || result === null) {
      return defaultValue as D;
    }
    result = (result as Record<string, unknown>)[key];
  }

  return result === undefined ? (defaultValue as D) : (result as T);
}

/**
 * Creates a new object with only the specified keys
 * @param obj - Original object
 * @param keys - Keys to include in the new object
 */
export function pick<T extends object, K extends keyof T>(obj: T, keys: K[]): Pick<T, K> {
  return keys.reduce(
    (result, key) => {
      if (Object.prototype.hasOwnProperty.call(obj, key)) {
        result[key] = obj[key];
      }
      return result;
    },
    {} as Pick<T, K>
  );
}

/**
 * Creates a new object excluding the specified keys
 * @param obj - Original object
 * @param keys - Keys to exclude from the new object
 */
export function omit<T extends object, K extends keyof T>(obj: T, keys: K[]): Omit<T, K> {
  const result = { ...obj };
  keys.forEach(key => {
    delete result[key];
  });
  return result;
}

/**
 * Checks if an object is empty
 * @param obj - Object to check
 */
export function isEmptyObject(obj: object): boolean {
  return Object.keys(obj).length === 0;
}

/**
 * Merges two objects deeply
 * @param target - Target object
 * @param source - Source object
 */
export function deepMerge<T extends object, S extends object>(target: T, source: S): T & S {
  const output = { ...target } as T & S;

  if (isObject(target) && isObject(source)) {
    Object.keys(source).forEach(key => {
      if (isObject(source[key as keyof S])) {
        if (!(key in target)) {
          Object.assign(output, { [key]: source[key as keyof S] });
        } else {
          const merged = deepMerge(
            target[key as keyof T] as object,
            source[key as keyof S] as object
          );
          (output as Record<string, unknown>)[key] = merged;
        }
      } else {
        Object.assign(output, { [key]: source[key as keyof S] });
      }
    });
  }

  return output;
}

/**
 * Helper: Checks if value is an object
 */
function isObject(item: unknown): item is object {
  return item !== null && typeof item === 'object' && !Array.isArray(item);
}

/**
 * Stable JSON stringify: sorts object keys recursively to ensure deterministic output
 * Useful for creating cache keys and hashing payloads
 */
export function stableStringify(value: unknown): string {
  const seen = new WeakSet<object>();

  const stringifyInternal = (val: unknown): unknown => {
    if (val === null || typeof val !== 'object') return val;
    if (seen.has(val as object)) return '[Circular]';
    seen.add(val as object);

    if (Array.isArray(val)) {
      return (val as unknown[]).map(item => stringifyInternal(item));
    }

    const obj = val as Record<string, unknown>;
    const sortedKeys = Object.keys(obj).sort();
    const result: Record<string, unknown> = {};
    for (const key of sortedKeys) {
      result[key] = stringifyInternal(obj[key]);
    }
    return result;
  };

  return JSON.stringify(stringifyInternal(value));
}

/**
 * DJB2 string hash, returned as hex string
 * Lightweight and fast for client-side hashing of cache keys
 */
export function hashStringDjb2(str: string): string {
  let hash = 5381;
  for (let i = 0; i < str.length; i++) {
    hash = (hash * 33) ^ str.charCodeAt(i);
  }
  // Convert to unsigned 32-bit and hex
  return (hash >>> 0).toString(16);
}
