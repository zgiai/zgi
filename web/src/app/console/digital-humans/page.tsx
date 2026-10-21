import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';
import { DigitalHumanList } from '@/components/digital-human/digital-human-list';

export default function DigitalHumansPage() {
  return (
    <DigitalHumanFeatureGate>
      <DigitalHumanList />
    </DigitalHumanFeatureGate>
  );
}
