'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { DigitalHumanReports } from '@/components/digital-human/digital-human-reports';

export default function DigitalHumanReportsPage() {
  const params = useParams<{ digitalHumanId: string }>();
  return (
    <DigitalHumanFeatureGate permission="digital_human.view">
      <DigitalHumanReports id={params.digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
