import { redirect } from 'next/navigation';

interface LegacyDigitalHumanPreferencesPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function LegacyDigitalHumanPreferencesPage({
  params,
}: LegacyDigitalHumanPreferencesPageProps) {
  const { digitalHumanId } = await params;
  redirect(`/console/digital-humans/${digitalHumanId}/configuration?section=behavior`);
}
