'use client'

import DoughnutChart from '@/components/charts/doughnut-chart'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

export default function AnalyticsCard08() {

  const chartData = {
    labels: ['Desktop', 'Mobile', 'Tablet'],
    datasets: [
      {
        label: 'Sessions By Device',
        data: [
          12, 50, 38,
        ],
        backgroundColor: [
          tailwindConfig.theme.colors.violet[500],
          tailwindConfig.theme.colors.sky[500],
          tailwindConfig.theme.colors.violet[800],
        ],
        hoverBackgroundColor: [
          tailwindConfig.theme.colors.violet[600],
          tailwindConfig.theme.colors.sky[600],
          tailwindConfig.theme.colors.violet[900],
        ],
        borderWidth: 0,
      },
    ],
  }

  return (
    <div className="flex flex-col col-span-full sm:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Sessions By Device</h2>
      </header>
      {/* Chart built with Chart.js 3 */}
      {/* Change the height attribute to adjust the chart height */}
      <DoughnutChart data={chartData} width={389} height={260} />
    </div>
  )
}
