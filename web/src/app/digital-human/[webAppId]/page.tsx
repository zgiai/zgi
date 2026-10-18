'use client';

import { useParams } from 'next/navigation';
import { DigitalHumanWebApp } from '@/components/digital-human/digital-human-webapp';

export default function DigitalHumanWebAppPage() {
  const { webAppId } = useParams<{ webAppId: string }>();
  return <DigitalHumanWebApp webAppId={webAppId} />;
}
