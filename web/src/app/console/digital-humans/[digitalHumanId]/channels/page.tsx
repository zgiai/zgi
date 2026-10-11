'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanChannels } from '@/components/digital-human/digital-human-channels';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';

export default function DigitalHumanChannelsPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.view">
      <DigitalHumanChannels id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
