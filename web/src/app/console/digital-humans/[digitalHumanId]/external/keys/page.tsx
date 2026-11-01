'use client';

import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { useParams } from 'next/navigation';
import { DigitalHumanAPIKeys } from '@/components/digital-human/digital-human-api-keys';

export default function DigitalHumanAPIKeysPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.publish">
      <DigitalHumanAPIKeys id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
