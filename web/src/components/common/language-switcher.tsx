'use client';

import { useState, useEffect } from 'react';
import { useLocale } from '@/hooks/use-locale';
import { getLocaleLabel, type Locale } from '@/lib/i18n';
import { LANGUAGES } from '@/lib/constants';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Icons } from '@/components/ui/icons';
import { Check } from 'lucide-react';
import { useUpdateInterfaceLanguage } from '@/hooks/use-update-interface-language';
import { useAuthStore } from '@/store/auth-store';

export function LanguageSwitcher({ className }: { className?: string }) {
  const { locale, isEnabled, setLocale } = useLocale();
  const { mutate } = useUpdateInterfaceLanguage();
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const [isOpen, setIsOpen] = useState(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  // Don't render if language switching is disabled
  if (!isEnabled) {
    return null;
  }

  const trigger = (
    <Button variant="ghost" size="sm" className={className} aria-label="Switch language">
      <Icons.languages className="h-4 w-4" />
      <span className="hidden sm:inline-block">{getLocaleLabel(locale)}</span>
    </Button>
  );

  if (!mounted) {
    return trigger;
  }

  return (
    <DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
      <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40 space-y-1">
        {LANGUAGES.map(lang => (
          <DropdownMenuItem
            key={lang.value}
            onClick={async () => {
              if (isAuthenticated) {
                mutate(lang.value as Locale);
              } else {
                await setLocale(lang.value as Locale);
              }
              setIsOpen(false);
            }}
            className={`cursor-pointer ${locale === lang.value ? 'bg-accent' : ''}`}
          >
            <span className="flex items-center justify-between w-full">
              {lang.label}
              {locale === lang.value && <Check className="h-4 w-4" />}
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
