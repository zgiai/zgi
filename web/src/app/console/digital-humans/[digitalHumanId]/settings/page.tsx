'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanSettings } from '@/components/digital-human/digital-human-configuration';

export default function DigitalHumanSettingsPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanSettings id={params.digitalHumanId} />;
}
