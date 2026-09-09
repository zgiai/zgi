'use client';

import { useRef } from 'react';
import { MDXProvider } from '@mdx-js/react';
import DigitalHumanApiDocsEn from '@/components/digital-human/md/digital-human-api.en-US.mdx';
import DigitalHumanApiDocsZh from '@/components/digital-human/md/digital-human-api.zh-Hans.mdx';
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
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { API_URL } from '@/lib/config';

export function DigitalHumanAPIDocs() {
  const contentRef = useRef<HTMLDivElement>(null);
  const { locale } = useLocale();
  const t = useT('digitalHumans.access');
  const Docs = locale === 'zh-Hans' ? DigitalHumanApiDocsZh : DigitalHumanApiDocsEn;
  const apiBase = `${API_URL}/api/v1`;
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

  return (
    <div className="@container/api-docs p-4">
      <ApiDocsProvider apibase={apiBase}>
        <div className="mx-auto mb-3 flex w-full max-w-[1360px] justify-end">
          <Button variant="outline" size="sm" asChild>
            <a href={`${API_URL}/api/openapi.yaml`} target="_blank" rel="noreferrer">
              {t('openAPIDocument')}
            </a>
          </Button>
        </div>
        <div className="mx-auto grid w-full max-w-[1360px] items-start gap-8 @5xl/api-docs:grid-cols-[minmax(0,1fr)_280px]">
          <div ref={contentRef} className="prose min-w-0 max-w-5xl text-[14px] dark:prose-invert">
            <MDXProvider components={components}>
              <Docs components={components} />
            </MDXProvider>
          </div>
          <div className="hidden self-stretch @5xl/api-docs:block">
            <FloatingToc rootRef={contentRef} />
          </div>
        </div>
      </ApiDocsProvider>
    </div>
  );
}
