import { redirectToConsoleMarket } from '../../redirect';

// Deployments configured with the old OAuth return address keep working: the authorization's
// state and code are handed on unchanged.
export default async function OldZgiMarketOAuthCallbackPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  redirectToConsoleMarket('oauth/callback', await searchParams);
}
