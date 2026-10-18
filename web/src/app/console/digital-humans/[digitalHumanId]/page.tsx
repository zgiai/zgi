import { DigitalHumanDashboard } from '@/components/digital-human/digital-human-dashboard';

interface DigitalHumanDetailPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function DigitalHumanDetailPage({ params }: DigitalHumanDetailPageProps) {
  const { digitalHumanId } = await params;
  return <DigitalHumanDashboard id={digitalHumanId} />;
}
