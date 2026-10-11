'use client';

import { useSearchParams } from 'next/navigation';
import {
  DigitalHumanConfigurationEditor,
  type DigitalHumanConfigurationPanel,
} from '@/components/digital-human/digital-human-configuration';

const legacySectionPanels: Partial<Record<string, DigitalHumanConfigurationPanel>> = {
  behavior: 'response',
};

export function DigitalHumanConfigurationHub({ id }: { id: string }) {
  const searchParams = useSearchParams();
  const legacySection = searchParams.get('section');
  const initialPanel = legacySection ? legacySectionPanels[legacySection] : null;

  return (
    <div className="mx-auto h-full min-h-0 w-full max-w-[2400px]">
      <DigitalHumanConfigurationEditor id={id} initialPanel={initialPanel} />
    </div>
  );
}
