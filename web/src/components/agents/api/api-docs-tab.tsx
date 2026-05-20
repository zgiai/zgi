'use client';

import React, { useRef } from 'react';
import { AgentType } from '@/services/types/agent';
import WorkflowApiDocs from '@/components/agents/api/md/workflow-api.mdx';
import ChatWorkflowApiDocs from '@/components/agents/api/md/chat-workflow-api.mdx';
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

interface ApiDocsTabProps {
  agentType: AgentType | null | undefined;
}

export default function ApiDocsTab({ agentType }: ApiDocsTabProps) {
  const contentRef = useRef<HTMLDivElement>(null);

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

  const renderDocs = () =>
    agentType === AgentType.WORKFLOW ? (
      <WorkflowApiDocs components={components} />
    ) : (
      <ChatWorkflowApiDocs components={components} />
    );

  return (
    <div className="p-4 space-y-4">
      <ApiDocsProvider apibase={API_URL + '/v1/api'}>
        <div ref={contentRef} className="prose dark:prose-invert max-w-5xl mx-auto text-[14px]">
          <MDXProvider components={components}>{renderDocs()}</MDXProvider>
        </div>
      </ApiDocsProvider>
      <FloatingToc rootRef={contentRef} />
    </div>
  );
}
