import { redirect } from 'next/navigation';

interface LegacyDigitalHumanSettingsPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function LegacyDigitalHumanSettingsPage({
  params,
}: LegacyDigitalHumanSettingsPageProps) {
  const { digitalHumanId } = await params;
  redirect(`/console/digital-humans/${digitalHumanId}/configuration?section=identity`);
}
