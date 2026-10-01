'use client';
import { use } from 'react';
import { PersonalConnectionsPage } from '@/components/connectors/connections/personal-connections-page';

export default function MailPage({
  searchParams,
}: {
  searchParams: Promise<{ provider?: string }>;
}) {
  const { provider } = use(searchParams);
  return <PersonalConnectionsPage mailOnly preferredProvider={provider} />;
}
