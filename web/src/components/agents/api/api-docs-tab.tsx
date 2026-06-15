'use client';

import React, { useRef } from 'react';
import { AgentType } from '@/services/types/agent';
import AgentApiDocsEn from '@/components/agents/api/md/agent-api.en-US.mdx';
import AgentApiDocsZh from '@/components/agents/api/md/agent-api.zh-Hans.mdx';
import WorkflowApiDocsEn from '@/components/agents/api/md/workflow-api.en-US.mdx';
import WorkflowApiDocsZh from '@/components/agents/api/md/workflow-api.zh-Hans.mdx';
import ChatWorkflowApiDocsEn from '@/components/agents/api/md/chat-workflow-api.en-US.mdx';
import ChatWorkflowApiDocsZh from '@/components/agents/api/md/chat-workflow-api.zh-Hans.mdx';
import { MDXProvider } from '@mdx-js/react';
import { CodeGroup } from '@/components/agents/api/ui/code-group';
import MDComponents, {
  H1,
  H2,
  H3,
  H4,
  H5,
  H6,
  Ul,
  Ol,
  Li,
  InlineCode,
} from '@/components/agents/api/ui/md';
import { FloatingToc } from '@/components/agents/api/ui/floating-toc';
import { ApiDocsProvider } from '@/components/agents/api/ui/api-docs-context';
import { API_URL } from '@/lib/config';
import { useLocale } from '@/hooks/use-locale';

interface ApiDocsTabProps {
  agentType: AgentType | string | null | undefined;
}

function normalizeAgentType(agentType: ApiDocsTabProps['agentType']) {
  return String(agentType ?? '')
    .trim()
    .toUpperCase()
    .replace(/-/g, '_');
}

export default function ApiDocsTab({ agentType }: ApiDocsTabProps) {
  const contentRef = useRef<HTMLDivElement>(null);
  const { locale } = useLocale();

  const components = {
    CodeGroup,
    Row: MDComponents.Row,
    Col: MDComponents.Col,
    Heading: MDComponents.Heading,
    Properties: MDComponents.Properties,
    Property: MDComponents.Property,
    SubProperty: MDComponents.SubProperty,
    p: MDComponents.Paragraph,
    h1: H1,
    h2: H2,
    h3: H3,
    h4: H4,
    h5: H5,
    h6: H6,
    ul: Ul,
    ol: Ol,
    li: Li,
    code: InlineCode,
  };

  const normalizedAgentType = normalizeAgentType(agentType);
  const isAgentRuntime = normalizedAgentType === AgentType.AGENT;
  const apiBase = API_URL + '/api/v1';

  const renderDocs = () => {
    if (isAgentRuntime) {
      const AgentApiDocs = locale === 'zh-Hans' ? AgentApiDocsZh : AgentApiDocsEn;
      return <AgentApiDocs components={components} />;
    }
    if (normalizedAgentType === AgentType.WORKFLOW) {
      const WorkflowApiDocs = locale === 'zh-Hans' ? WorkflowApiDocsZh : WorkflowApiDocsEn;
      return <WorkflowApiDocs components={components} />;
    }
    const ChatWorkflowApiDocs =
      locale === 'zh-Hans' ? ChatWorkflowApiDocsZh : ChatWorkflowApiDocsEn;
    return <ChatWorkflowApiDocs components={components} />;
  };

  return (
    <div className="p-4 space-y-4">
      <ApiDocsProvider apibase={apiBase}>
        <div ref={contentRef} className="prose dark:prose-invert max-w-5xl mx-auto text-[14px]">
          <MDXProvider components={components}>{renderDocs()}</MDXProvider>
        </div>
      </ApiDocsProvider>
      <FloatingToc rootRef={contentRef} />
    </div>
  );
}
