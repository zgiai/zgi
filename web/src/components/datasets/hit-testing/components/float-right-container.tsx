'use client';

import React from 'react';
import { useT } from '@/i18n';
import { X, ChevronLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import type { FloatRightContainerProps } from '../types';

/**
 * FloatRightContainer Component
 * Responsive container that shows as a fixed right panel on desktop
 * and as a drawer/sheet on mobile devices
 */
export function FloatRightContainer({
  children,
  isShow,
  onToggle,
  isMobile = false,
}: FloatRightContainerProps) {
  const t = useT('datasets');

  // Desktop: Fixed right panel
  if (!isMobile) {
    return (
      <div
        className={cn(
          'fixed right-0 top-16 h-[calc(100vh-4rem)] bg-background border-l border-border transition-transform duration-300 ease-in-out z-20',
          'max-w-[calc(50%-4px)] w-full md:w-[600px]',
          isShow ? 'translate-x-0' : 'translate-x-full'
        )}
      >
        {/* Header */}
        <div className="flex items-center justify-between p-4 border-b border-border">
          <h3 className="font-semibold text-lg">{t('hitTesting.results')}</h3>
          <Button variant="ghost" size="sm" onClick={onToggle} className="h-8 w-8 p-0">
            <X className="h-4 w-4" />
          </Button>
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-4">{children}</div>
      </div>
    );
  }

  // Mobile: Sheet/Drawer
  return (
    <Sheet open={isShow} onOpenChange={onToggle}>
      <SheetTrigger asChild>
        <Button variant="outline" size="sm" className="fixed bottom-4 right-4 z-50 shadow-lg">
          {t('hitTesting.viewResults')}
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <ChevronLeft className="h-4 w-4" />
            {t('hitTesting.results')}
          </SheetTitle>
        </SheetHeader>
        <div className="mt-6 overflow-y-auto">{children}</div>
      </SheetContent>
    </Sheet>
  );
}
