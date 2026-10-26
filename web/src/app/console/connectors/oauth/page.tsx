'use client';

import { use } from 'react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ConnectorOAuthFlowStatus } from '@/components/connectors/setup/connector-oauth-flow-status';
import { useT } from '@/i18n';

export default function ConnectorOAuthCompletionPage({
  searchParams,
}: {
  searchParams: Promise<{ flow_id?: string }>;
}) {
  const t = useT('connectors');
  const { flow_id: flowId } = use(searchParams);
  const valid = Boolean(
    flowId && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(flowId)
  );
  return (
    <main className="mx-auto w-full max-w-lg space-y-4 p-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('oauth.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {valid ? (
            <ConnectorOAuthFlowStatus flowId={flowId ?? ''} />
          ) : (
            <p role="alert">{t('oauth.failed')}</p>
          )}
          <p className="text-sm text-muted-foreground">{t('oauth.returnToWindow')}</p>
          <Button variant="outline" onClick={() => window.close()}>
            {t('reconnect.close')}
          </Button>
          <Button asChild variant="ghost">
            <Link href="/console/connectors/connections">{t('connections.title')}</Link>
          </Button>
        </CardContent>
      </Card>
    </main>
  );
}
