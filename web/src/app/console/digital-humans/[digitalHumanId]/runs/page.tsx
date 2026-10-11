'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { DigitalHumanRuns } from '@/components/digital-human/digital-human-runs';

export default function DigitalHumanRunsPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.logs.view">
      <DigitalHumanRuns id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
