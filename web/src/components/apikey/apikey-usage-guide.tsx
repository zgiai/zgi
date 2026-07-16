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
 * @usage Use on the API key management page to provide OpenAI- and Anthropic-compatible access guidance
 * @example
 * <ApiKeyUsageGuide />
 */
export function ApiKeyUsageGuide({ apiBaseUrl = API_URL }: ApiKeyUsageGuideProps): JSX.Element {
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
      `- ${t('usage.markdown.responsesLabel')}: \`${openAiBaseUrl}/responses\``,
      `- ${t('usage.markdown.anthropicBaseLabel')}: \`${openAiBaseUrl}\``,
      `- ${t('usage.markdown.anthropicMessagesLabel')}: \`${openAiBaseUrl}/messages\``,
      '',
      `> ${t('usage.markdown.supportNote')}`,
    ].join('\n');
  }, [apiBaseUrl, openAiBaseUrl, t]);

  const curlExampleMarkdown = React.useMemo(() => {
    return [
      `## ${t('usage.markdown.exampleTitle')}`,
      '',
      `### ${t('usage.markdown.openAiCurlTitle')}`,
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
      `### ${t('usage.markdown.openAiResponsesCurlTitle')}`,
      '',
      '```bash',
      `curl --request POST '${openAiBaseUrl}/responses' \\`,
      "  --header 'Content-Type: application/json' \\",
      "  --header 'Authorization: Bearer YOUR_API_KEY' \\",
      "  --data '{",
      '    "model": "your-model-name",',
      '    "input": "Hello!"',
      "  }'",
      '```',
      '',
      `### ${t('usage.markdown.anthropicCurlTitle')}`,
      '',
      '```bash',
      `curl --request POST '${openAiBaseUrl}/messages' \\`,
      "  --header 'Content-Type: application/json' \\",
      "  --header 'x-api-key: YOUR_API_KEY' \\",
      "  --header 'anthropic-version: 2023-06-01' \\",
      "  --data '{",
      '    "model": "your-model-name",',
      '    "max_tokens": 1024,',
      '    "messages": [',
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
      <div className="rounded-lg border bg-muted/20 px-3 py-2">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
            <div className="inline-flex items-center gap-1.5 rounded-full border bg-background px-2.5 py-1 text-xs font-medium text-foreground/80">
              <Sparkles className="size-3.5 text-primary" />
              {t('usage.badge')}
            </div>
            <span className="text-muted-foreground">{t('usage.apiBaseLabel')}</span>
            <code className="inline-block max-w-full truncate rounded-md border bg-background px-2 py-1 font-mono text-xs">
              {apiBaseUrl}
            </code>
          </div>
          <Button variant="outline" size="sm" onClick={() => setDialogOpen(true)}>
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
