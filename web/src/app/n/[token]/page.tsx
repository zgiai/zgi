import { AnnouncementPageClient } from './page-client';

interface AnnouncementPageProps {
  params: Promise<{ token: string }> | { token: string };
}

export default async function AnnouncementPage({ params }: AnnouncementPageProps) {
  const resolvedParams = await params;
  return <AnnouncementPageClient token={resolvedParams.token} />;
}
