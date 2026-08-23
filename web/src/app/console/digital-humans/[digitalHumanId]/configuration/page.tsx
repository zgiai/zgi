'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanConfigurationHub } from '@/components/digital-human/digital-human-configuration-hub';

export default function DigitalHumanConfigurationPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanConfigurationHub id={params.digitalHumanId} />;
}
