export function getPublicRuntimeEnv(): Record<string, string> {
  const result: Record<string, string> = {};
  const env = process.env as Record<string, string | undefined>;
  const keys = Object.keys(env);
  for (const key of keys) {
    if (key.startsWith('NEXT_PUBLIC_')) {
      const val = env[key];
      if (typeof val === 'string' && val.length > 0) {
        result[key] = val;
      }
    }
  }
  return result;
}
