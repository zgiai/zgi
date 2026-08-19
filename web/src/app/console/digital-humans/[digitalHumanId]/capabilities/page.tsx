'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanCapabilities } from '@/components/digital-human/digital-human-configuration';

export default function DigitalHumanCapabilitiesPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanCapabilities id={params.digitalHumanId} />;
}
