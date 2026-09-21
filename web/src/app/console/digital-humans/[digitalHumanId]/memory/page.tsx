'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanMemory } from '@/components/digital-human/digital-human-memory';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';

export default function DigitalHumanMemoryPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.memory.view">
      <DigitalHumanMemory id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
