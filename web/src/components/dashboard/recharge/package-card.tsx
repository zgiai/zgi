'use client';

import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';

interface PackageCardProps {
  id: string;
  name: string;
  points: string;
  price: string;
  productCode: string;
  badge?: string;
  isLoading?: boolean;
  onBuy?: () => void;
  isBuying?: boolean;
}

export function PackageCard({
  name,
  points,
  price,
  badge,
  isLoading,
  onBuy,
  isBuying,
}: PackageCardProps) {
  const t = useT('dashboard');

  if (isLoading) {
    return (
      <Card className="relative">
        <CardContent className="pt-6">
          <Skeleton className="h-6 w-24 mb-2" />
          <Skeleton className="h-6 w-32 mb-1" />
          <Skeleton className="h-6 w-20 mb-3" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="relative hover:shadow-lg transition-shadow">
      <CardContent className="pt-6">
        {badge && (
          <span className="absolute top-2 right-2 text-xs bg-red-500 text-white px-2 py-1 rounded">
            {badge}
          </span>
        )}
        <div className="text-lg font-bold mb-2">{name}</div>
        <div className="text-lg font-bold text-blue-600 mb-1">{points}</div>
        <div className="text-lg font-bold text-primary mb-3">{price}</div>
        <Button
          className="w-full bg-blue-600 hover:bg-blue-700"
          onClick={onBuy}
          disabled={isBuying}
        >
          {t('costCenter.packages.buyNow')}
        </Button>
      </CardContent>
    </Card>
  );
}
