import { redirect } from 'next/navigation';

export default async function LegacyIntegrationDetailPage({
  params,
}: {
  params: Promise<{ integrationId: string }>;
}) {
  const { integrationId } = await params;
  redirect(
    `/console/integrations?view=available&integration_id=${encodeURIComponent(integrationId)}`
  );
}
