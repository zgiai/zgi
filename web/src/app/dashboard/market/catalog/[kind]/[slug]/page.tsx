import { redirectToConsoleMarket } from '../../../redirect';

// The market moved to the console; old links, such as an OAuth return address, still land on it.
export default async function OldZgiMarketListingPage({
  params,
  searchParams,
}: {
  params: Promise<{ kind: string; slug: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { kind, slug } = await params;
  redirectToConsoleMarket(
    `catalog/${encodeURIComponent(kind)}/${encodeURIComponent(slug)}`,
    await searchParams
  );
}
