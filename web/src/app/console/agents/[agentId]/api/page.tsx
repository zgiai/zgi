'use client';

import { useParams } from 'next/navigation';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import ApiKeysTab from '@/components/agents/api/api-keys-tab';
import ApiDocsTab from '@/components/agents/api/api-docs-tab';
import { useAgent } from '@/hooks/agent/use-agents';
import type { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';

export default function AgentApiPage() {
  const { agentId } = useParams<{ agentId: string }>();
  const t = useT();

  const { agent } = useAgent(agentId);
  const agentType = (agent?.data?.agent_type as AgentType | undefined) ?? undefined;

  return (
    <div className="p-4 space-y-4">
      <Tabs defaultValue="api-keys" className="w-full">
        <TabsList>
          <TabsTrigger value="api-keys">{t('agents.apiKeys.navTitle')}</TabsTrigger>
          <TabsTrigger value="api-docs">{t('agents.apiTitle')}</TabsTrigger>
        </TabsList>
        <TabsContent value="api-keys">
          <ApiKeysTab agentId={agentId} />
        </TabsContent>
        <TabsContent value="api-docs">
          <ApiDocsTab agentType={agentType} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
