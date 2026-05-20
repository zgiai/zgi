'use client';

import Link from 'next/link';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { ROUTES } from '@/constants/routes';
import { Home } from 'lucide-react';

export default function NotFound() {
  const t = useT();

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-background px-4">
      <div className="text-center">
        <h1 className="text-9xl font-bold text-primary">{t('common.errorPages.notFound.title')}</h1>
        <h2 className="mt-4 text-2xl font-semibold text-foreground">
          {t('common.errorPages.notFound.subtitle')}
        </h2>
        <p className="mt-2 text-muted-foreground">{t('common.errorPages.notFound.description')}</p>
        <div className="mt-8">
          <Button asChild>
            <Link href={ROUTES.CONSOLE.HOME}>
              <Home className="mr-2 h-4 w-4" />
              {t('common.errorPages.notFound.backHome')}
            </Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
