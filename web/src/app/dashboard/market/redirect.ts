import { redirect } from 'next/navigation';

type Query = Record<string, string | string[] | undefined>;

/** Sends an old market address under /dashboard to the same page under /console, query kept. */
export function redirectToConsoleMarket(path: string, query: Query): never {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    for (const item of Array.isArray(value) ? value : value === undefined ? [] : [value]) {
      search.append(key, item);
    }
  }
  const suffix = search.toString();
  redirect(`/console/market/${path}${suffix ? `?${suffix}` : ''}`);
}
