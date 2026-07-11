'use client';

import { use, useEffect } from 'react';
import { usePathname, useRouter } from 'next/navigation';
import { Loader2 } from 'lucide-react';

interface AgentApiIndexPageProps {
  params: Promise<{ agentId: string }>;
}

export default function AgentApiIndexPage({ params }: AgentApiIndexPageProps) {
  const { agentId } = use(params);
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    const basePath = pathname?.startsWith('/console/workflows')
      ? `/console/workflows/${agentId}`
      : `/console/agents/${agentId}`;
    router.replace(`${basePath}/api/keys`);
  }, [agentId, pathname, router]);

  return (
    <div className="flex h-full w-full items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  );
}
