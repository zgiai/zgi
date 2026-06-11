'use client';

import { use, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';

interface AgentApiIndexPageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentApiIndexPage({ params }: AgentApiIndexPageProps) {
  const { agentId } = use(params);
  const router = useRouter();

  useEffect(() => {
    router.replace(`/console/agents/${agentId}/api/keys`);
  }, [agentId, router]);

  return (
    <div className="flex h-full w-full items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}
