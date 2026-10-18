'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanExternalAccess } from '@/components/digital-human/digital-human-external-access';

export default function DigitalHumanExternalAccessPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanExternalAccess id={params.digitalHumanId} />;
}
