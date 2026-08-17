import { redirect } from 'next/navigation';

type LegacyIntegrationSearchParams = Promise<Record<string, string | string[] | undefined>>;

function legacyView(value: string | string[] | undefined) {
  const tab = Array.isArray(value) ? value[0] : value;
  switch (tab) {
    case 'connections':
      return 'connected';
    case 'policies':
      return 'policies';
    case 'executions':
      return 'executions';
    default:
      return 'available';
  }
}

export default async function LegacyOrganizationIntegrationsPage({
  searchParams,
}: {
  searchParams: LegacyIntegrationSearchParams;
}) {
  const legacyParams = await searchParams;
  const nextParams = new URLSearchParams({ view: legacyView(legacyParams.tab) });
  const integrationID = Array.isArray(legacyParams.integration_id)
    ? legacyParams.integration_id[0]
    : legacyParams.integration_id;
  if (integrationID) nextParams.set('integration_id', integrationID);
  redirect(`/console/integrations?${nextParams.toString()}`);
}
