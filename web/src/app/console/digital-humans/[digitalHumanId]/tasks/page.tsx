'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { DigitalHumanTasks } from '@/components/digital-human/digital-human-tasks';

export default function DigitalHumanTasksPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.view">
      <DigitalHumanTasks id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
