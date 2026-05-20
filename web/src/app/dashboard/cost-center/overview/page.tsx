'use client';

import { useT } from '@/i18n/translations';
import { WalletCard } from '@/components/dashboard/cost-center/overview/wallet-card';
import { PackagesSection } from '@/components/dashboard/cost-center/overview/packages-section';
import { PointsCard } from '@/components/dashboard/cost-center/overview/points-card';
import { useCloudOnlyPage } from '@/hooks/use-cloud-only-page';

export default function CostCenterOverviewPage() {
  const isCloud = useCloudOnlyPage();
  const t = useT('dashboard');

  if (!isCloud) {
    return null;
  }

  return (
    <div className="p-4 space-y-5 overflow-y-auto h-full">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold mb-1">{t('costCenter.title')}</h1>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {/* Wallet Balance */}
        <WalletCard />

        {/* AI Virtual Points */}
        <PointsCard />
      </div>

      {/* AI Packages Section */}
      <PackagesSection />
    </div>
  );
}
