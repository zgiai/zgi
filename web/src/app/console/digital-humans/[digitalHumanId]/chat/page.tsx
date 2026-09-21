import { DigitalHumanChat } from '@/components/digital-human/digital-human-chat';
import { DigitalHumanFeatureGate } from '@/components/digital-human/feature-gate';

interface DigitalHumanChatPageProps {
  params: Promise<{ digitalHumanId: string }>;
}

export default async function DigitalHumanChatPage({ params }: DigitalHumanChatPageProps) {
  const { digitalHumanId } = await params;
  return (
    <DigitalHumanFeatureGate permission="digital_human.run">
      <DigitalHumanChat id={digitalHumanId} />
    </DigitalHumanFeatureGate>
  );
}
