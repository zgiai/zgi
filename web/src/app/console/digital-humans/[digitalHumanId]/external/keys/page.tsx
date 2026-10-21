'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanAPIKeys } from '@/components/digital-human/digital-human-api-keys';

export default function DigitalHumanAPIKeysPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return <DigitalHumanAPIKeys id={params.digitalHumanId} />;
}
