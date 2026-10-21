'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { Button } from '@/components/ui/button';
import { marketSameOrigin } from '@/components/zgi-market/market-format';
import { useT } from '@/i18n';
import { zgiMarketService } from '@/services/zgi-market.service';

/**
 * The address the connector vendor sends the browser back to. It hands the vendor's answer to the
 * market through this instance and opens the page the market names, usually the listing page the
 * authorization started from. A page of another site is not opened: the failure is shown here.
 */
export default function ZgiMarketOAuthCallbackPage() {
  const t = useT();
  const [failed, setFailed] = useState(false);
  const started = useRef(false);

  useEffect(() => {
    // The state can be used once; React may run this effect twice in development.
    if (started.current) {
      return;
    }
    started.current = true;
    const query = window.location.search;
    // Keep the code and the state out of the history and the Referer of the next page.
    window.history.replaceState(null, '', window.location.pathname);
    zgiMarketService
      .completeOAuth(query)
      .then(redirect => {
        if (!marketSameOrigin(redirect, window.location.origin)) {
          setFailed(true);
          return;
        }
        window.location.replace(redirect);
      })
      .catch(() => setFailed(true));
  }, []);

  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-sm text-muted-foreground">
      {failed ? (
        <>
          <p>{t('market.catalog.oauth.failed')}</p>
          <Button asChild variant="outline" size="sm">
            <Link href="/console/market/catalog">{t('market.catalog.oauth.back')}</Link>
          </Button>
        </>
      ) : (
        <p>{t('market.catalog.oauth.completing')}</p>
      )}
    </div>
  );
}
