'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanChat } from '@/components/digital-human/digital-human-chat';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';

export default function DigitalHumanChatPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.run">
      <DigitalHumanChat id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
