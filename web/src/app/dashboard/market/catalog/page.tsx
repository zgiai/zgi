import { redirectToConsoleMarket } from '../redirect';

// The market moved to the console; old links still land on it.
export default async function OldZgiMarketCatalogPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  redirectToConsoleMarket('catalog', await searchParams);
}
