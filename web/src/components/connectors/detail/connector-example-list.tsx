'use client';

import { MessageSquareText } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useT } from '@/i18n';

export function ConnectorExampleList({ examples }: { examples: string[] }) {
  const t = useT('connectors');
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('detail.examplesTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        {examples.length > 0 ? (
          <ul className="space-y-3" role="list">
            {examples.map((example, index) => (
              <li key={`${index}-${example}`} className="flex items-start gap-3 text-sm leading-6">
                <MessageSquareText
                  aria-hidden="true"
                  className="mt-1 size-4 shrink-0 text-muted-foreground"
                />
                <span>{example}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">{t('detail.noExamples')}</p>
        )}
      </CardContent>
    </Card>
  );
}
