'use client';

import { useEffect } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Skeleton } from '@/components/ui/skeleton';
import { useWebAppConfig } from '@/hooks/webapp/use-webapp';
import { detectWebappMode } from '@/utils/webapp/helpers';

export default function WebappConversationRedirectPage(): JSX.Element {
  const { version_uuid, conversation_id } = useParams<{
    version_uuid: string;
    conversation_id: string;
  }>();
  const router = useRouter();
  const { data, isLoading } = useWebAppConfig(version_uuid);

  useEffect(() => {
    if (isLoading || !data?.data) return;
    const mode = detectWebappMode(data.data);
    const conv = encodeURIComponent(conversation_id);
    router.replace(`/webapp/${version_uuid}/${mode}?convId=${conv}`);
  }, [conversation_id, data, isLoading, router, version_uuid]);

  return (
    <div className="h-full w-full p-4">
      <Skeleton className="h-full w-full" />
    </div>
  );
}
