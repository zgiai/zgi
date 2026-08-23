import { redirect } from 'next/navigation';

interface LegacyDigitalHumanCapabilitiesPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function LegacyDigitalHumanCapabilitiesPage({
  params,
}: LegacyDigitalHumanCapabilitiesPageProps) {
  const { digitalHumanId } = await params;
  redirect(`/console/digital-humans/${digitalHumanId}/configuration?section=capabilities`);
}
