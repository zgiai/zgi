'use client';

import { useRef } from 'react';
import { MDXProvider } from '@mdx-js/react';
import RuntimeWorkspaceApiDocsEn from './runtime-workspace-api.en-US.mdx';
import RuntimeWorkspaceApiDocsZh from './runtime-workspace-api.zh-Hans.mdx';
import { ApiDocsProvider } from '@/components/agents/api/ui/api-docs-context';
import { CodeGroup } from '@/components/agents/api/ui/code-group';
import { FloatingToc } from '@/components/agents/api/ui/floating-toc';
import MDComponents, {
  H1,
  H2,
  H3,
  H4,
  H5,
  H6,
  InlineCode,
  Li,
  Ol,
  Ul,
} from '@/components/agents/api/ui/md';
import { Button } from '@/components/ui/button';
import { RuntimePageHeader } from '@/features/runtime-workspaces/runtime-page-header';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { API_URL } from '@/lib/config';

export function RuntimeWorkspaceAPIDocs() {
  const contentRef = useRef<HTMLDivElement>(null);
  const { locale } = useLocale();
  const t = useT('workspace');
  const Docs = locale === 'zh-Hans' ? RuntimeWorkspaceApiDocsZh : RuntimeWorkspaceApiDocsEn;
  const apiBase = `${API_URL}/api/connect/v1`;
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
    table: MDComponents.Table,
    thead: MDComponents.TableHead,
    tbody: MDComponents.TableBody,
    tr: MDComponents.TableRow,
    th: MDComponents.TableHeaderCell,
    td: MDComponents.TableCell,
  };

  return (
    <div className="@container/api-docs mx-auto w-full max-w-7xl px-4 py-5 @md/console:px-6 @md/console:py-6">
      <ApiDocsProvider apibase={apiBase}>
        <div className="mb-6">
          <RuntimePageHeader
            title={t('runtime.tabs.docs')}
            description={t('runtime.docsPage.description')}
            actions={
              <Button variant="outline" size="sm" asChild>
                <a
                  href={`${API_URL}/api/runtime-workspace-openapi.yaml`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {t('runtime.integration.openApi')}
                </a>
              </Button>
            }
          />
        </div>
        <div className="grid w-full items-start gap-8 @4xl/api-docs:grid-cols-[minmax(0,1fr)_280px]">
          <div ref={contentRef} className="prose min-w-0 max-w-5xl text-[14px] dark:prose-invert">
            <MDXProvider components={components}>
              <Docs components={components} />
            </MDXProvider>
          </div>
          <div className="hidden self-stretch @4xl/api-docs:block">
            <FloatingToc rootRef={contentRef} />
          </div>
        </div>
      </ApiDocsProvider>
    </div>
  );
}
