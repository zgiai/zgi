import type { Metadata } from 'next';
import { ArtifactSharePageClient } from './page-client';

export const metadata: Metadata = { robots: { index: false, follow: false } };

interface ArtifactSharePageProps {
  params: Promise<{ token: string }> | { token: string };
}

export default async function ArtifactSharePage({ params }: ArtifactSharePageProps) {
  const resolvedParams = await params;
  return <ArtifactSharePageClient token={resolvedParams.token} />;
}
