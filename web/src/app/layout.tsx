import { getLocale, getMessages } from 'next-intl/server';
import type { Metadata } from 'next';
import { FaviconSync } from '@/components/common/favicon-sync';
import { Providers } from '@/providers';
import { I18nClientProvider } from '@/providers/i18n-client-provider';
import { APP_NAME, FAVICON_URL, withBasePath } from '@/lib/config';
import Script from 'next/script';
import { getPublicRuntimeEnv } from '@/lib/runtime-env';

import './globals.css';
import '../styles/index.css';

export const metadata: Metadata = {
  title: APP_NAME,
  ...(FAVICON_URL
    ? {
        icons: {
          icon: [{ url: FAVICON_URL }],
          shortcut: [{ url: FAVICON_URL }],
          apple: [{ url: FAVICON_URL }],
        },
      }
    : {}),
};

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html lang={locale} suppressHydrationWarning>
      <body className="font-sans antialiased">
        <I18nClientProvider
          initialLocale={locale as 'zh-Hans' | 'en-US'}
          initialMessages={messages}
        >
          {/* Inject build-time server env first (for SSR), then load /env.js to override with runtime values in SSG/ISR */}
          <Script
            id="env-inline"
            strategy="beforeInteractive"
            dangerouslySetInnerHTML={{
              __html: `window.__ENV__ = Object.assign({}, window.__ENV__ || {}, ${JSON.stringify(
                getPublicRuntimeEnv()
              )});`,
            }}
          />
          <Script src={withBasePath('/env.js')} strategy="beforeInteractive" />
          <FaviconSync faviconUrl={FAVICON_URL} />
          <Providers>{children}</Providers>
        </I18nClientProvider>
      </body>
    </html>
  );
}
