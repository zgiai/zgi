import { redirect } from 'next/navigation';

interface DigitalHumanDetailPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function DigitalHumanDetailPage({ params }: DigitalHumanDetailPageProps) {
  const { digitalHumanId } = await params;
  redirect(`/console/digital-humans/${digitalHumanId}/configuration`);
}
