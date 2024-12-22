'use client'

import BarChart03 from '@/components/charts/bar-chart-03'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

export default function AnalyticsCard03() {

  const chartData = {
    labels: [
      '12-01-2022', '01-01-2023', '02-01-2023',
      '03-01-2023', '04-01-2023', '05-01-2023',
    ],
    datasets: [
      // Stack
      {
        label: 'Direct',
        data: [
          5000, 4000, 4000, 3800, 5200, 5100,
        ],
        backgroundColor: tailwindConfig.theme.colors.violet[700],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[800],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
      // Stack
      {
        label: 'Referral',
        data: [
          2500, 2600, 4000, 4000, 4800, 3500,
        ],
        backgroundColor: tailwindConfig.theme.colors.violet[500],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[600],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
      // Stack
      {
        label: 'Organic Search',
        data: [
          2300, 2000, 3100, 2700, 1300, 2600,
        ],
        backgroundColor: tailwindConfig.theme.colors.violet[300],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[400],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
      // Stack
      {
        label: 'Social',
        data: [
          4800, 4200, 4800, 1800, 3300, 3500,
        ],
        backgroundColor: tailwindConfig.theme.colors.violet[100],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[200],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
    ],
  }

  return(
    <div className="flex flex-col col-span-full sm:col-span-6 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60 flex items-center">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Acquisition Channels</h2>
      </header>
      {/* Chart built with Chart.js 3 */}
      {/* Change the height attribute to adjust the chart height */}
      <BarChart03 data={chartData} width={595} height={248} />
    </div>
  )
}
