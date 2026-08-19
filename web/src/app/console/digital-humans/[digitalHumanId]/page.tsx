'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanOverview } from '@/components/digital-human/digital-human-overview';

export default function DigitalHumanOverviewPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanOverview id={params.digitalHumanId} />;
}
