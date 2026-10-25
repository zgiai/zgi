'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { DigitalHumanArtifacts } from '@/components/digital-human/digital-human-artifacts';

export default function DigitalHumanArtifactsPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.view">
      <DigitalHumanArtifacts id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
