'use client';

import React from 'react';
import { CircleHelp, Sparkles } from 'lucide-react';
import { useT } from '@/i18n';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { API_URL } from '@/lib/config';

export interface ApiKeyUsageGuideProps {
  apiBaseUrl?: string;
}

/**
 * @component ApiKeyUsageGuide
 * @category Feature
 * @status Stable
 * @description Shows API key access basics and language-specific examples for the API keys page
 * @usage Use on the API key management page to provide OpenAI-compatible access guidance
 * @example
 * <ApiKeyUsageGuide />
 */
export function ApiKeyUsageGuide({
  apiBaseUrl = API_URL,
}: ApiKeyUsageGuideProps): JSX.Element {
  const t = useT('apikeys');
  const [dialogOpen, setDialogOpen] = React.useState<boolean>(false);

  const openAiBaseUrl = `${apiBaseUrl}/v1`;

  const usageOverviewMarkdown = React.useMemo(() => {
    return [
      `## ${t('usage.markdown.overviewTitle')}`,
      '',
      t('usage.markdown.overviewBody'),
      '',
      `- ${t('usage.markdown.apiBaseLabel')}: \`${apiBaseUrl}\``,
      `- ${t('usage.markdown.openAiBaseLabel')}: \`${openAiBaseUrl}\``,
      `- ${t('usage.markdown.chatCompletionsLabel')}: \`${openAiBaseUrl}/chat/completions\``,
      '',
      `> ${t('usage.markdown.supportNote')}`,
    ].join('\n');
  }, [apiBaseUrl, openAiBaseUrl, t]);

  const curlExampleMarkdown = React.useMemo(() => {
    return [
      `## ${t('usage.markdown.exampleTitle')}`,
      '',
      '```bash',
      `curl --request POST '${openAiBaseUrl}/chat/completions' \\`,
      "  --header 'Content-Type: application/json' \\",
      "  --header 'Authorization: Bearer YOUR_API_KEY' \\",
      "  --data '{",
      '    "model": "your-model-name",',
      '    "messages": [',
      '      {',
      '        "role": "system",',
      '        "content": "You are a helpful assistant."',
      '      },',
      '      {',
      '        "role": "user",',
      '        "content": "Hello!"',
      '      }',
      '    ]',
      "  }'",
      '```',
      '',
      t('usage.markdown.exampleNote'),
    ].join('\n');
  }, [openAiBaseUrl, t]);

  const tsExampleMarkdown = React.useMemo(() => {
    return [
      `## ${t('usage.markdown.tsExampleTitle')}`,
      '',
      `${t('usage.markdown.tsInstallLabel')}:`,
      '',
      '```bash',
      'npm install openai',
      '```',
      '',
      '```ts',
      "import OpenAI from 'openai';",
      '',
      'const client = new OpenAI({',
      '  apiKey: process.env.PLATFORM_API_KEY,',
      `  baseURL: '${openAiBaseUrl}',`,
      '});',
      '',
      'const completion = await client.chat.completions.create({',
      "  model: 'your-model-name',",
      '  messages: [',
      "    { role: 'system', content: 'You are a helpful assistant.' },",
      "    { role: 'user', content: 'Hello!' },",
      '  ],',
      '});',
      '',
      'console.log(completion.choices[0]?.message?.content);',
      '```',
      '',
      t('usage.markdown.tsExampleNote'),
    ].join('\n');
  }, [openAiBaseUrl, t]);

  const pythonExampleMarkdown = React.useMemo(() => {
    return [
      `## ${t('usage.markdown.pythonExampleTitle')}`,
      '',
      `${t('usage.markdown.pythonInstallLabel')}:`,
      '',
      '```bash',
      'pip install openai',
      '```',
      '',
      '```python',
      'import os',
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      '    api_key=os.environ.get("PLATFORM_API_KEY"),',
      `    base_url='${openAiBaseUrl}',`,
      ')',
      '',
      'completion = client.chat.completions.create(',
      '    model="your-model-name",',
      '    messages=[',
      '        {"role": "system", "content": "You are a helpful assistant."},',
      '        {"role": "user", "content": "Hello!"},',
      '    ],',
      ')',
      '',
      'print(completion.choices[0].message.content)',
      '```',
      '',
      t('usage.markdown.pythonExampleNote'),
    ].join('\n');
  }, [openAiBaseUrl, t]);

  return (
    <>
      <div className="rounded-xl border bg-muted/30 p-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2 rounded-full border bg-background px-3 py-1 text-xs font-medium text-foreground/80">
              <Sparkles className="size-3.5 text-primary" />
              {t('usage.badge')}
            </div>
            <div className="space-y-1">
              <div className="text-sm font-medium">{t('usage.inlineTitle')}</div>
              <div className="text-sm leading-6 text-muted-foreground">
                {t('usage.inlineDescription')}
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="text-muted-foreground">{t('usage.apiBaseLabel')}</span>
              <code className="rounded-md border bg-background px-2 py-1 font-mono text-xs">
                {apiBaseUrl}
              </code>
            </div>
          </div>
          <Button variant="outline" onClick={() => setDialogOpen(true)}>
            <CircleHelp className="size-4" />
            {t('usage.openDialog')}
          </Button>
        </div>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent size="xl">
          <DialogHeader>
            <DialogTitle>{t('usage.dialogTitle')}</DialogTitle>
            <DialogDescription>{t('usage.dialogDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="pb-6">
            <div className="space-y-4">
              <div className="rounded-xl border bg-background p-4">
                <MarkdownViewer content={usageOverviewMarkdown} />
              </div>

              <Tabs defaultValue="curl" className="space-y-4">
                <TabsList className="grid w-full grid-cols-3">
                  <TabsTrigger value="curl">{t('usage.markdown.tabs.curl')}</TabsTrigger>
                  <TabsTrigger value="ts">{t('usage.markdown.tabs.ts')}</TabsTrigger>
                  <TabsTrigger value="python">{t('usage.markdown.tabs.python')}</TabsTrigger>
                </TabsList>

                <TabsContent value="curl" className="mt-0">
                  <div className="rounded-xl border bg-background p-4">
                    <MarkdownViewer content={curlExampleMarkdown} />
                  </div>
                </TabsContent>

                <TabsContent value="ts" className="mt-0">
                  <div className="rounded-xl border bg-background p-4">
                    <MarkdownViewer content={tsExampleMarkdown} />
                  </div>
                </TabsContent>

                <TabsContent value="python" className="mt-0">
                  <div className="rounded-xl border bg-background p-4">
                    <MarkdownViewer content={pythonExampleMarkdown} />
                  </div>
                </TabsContent>
              </Tabs>
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
