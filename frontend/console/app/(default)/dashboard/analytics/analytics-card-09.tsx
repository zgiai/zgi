'use client'

import DoughnutChart from '@/components/charts/doughnut-chart'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

export default function AnalyticsCard09() {

  const chartData = {
    labels: ['<18', '18-24', '24-36', '>35'],
    datasets: [
      {
        label: 'Visit By Age Category',
        data: [
          30, 50, 5, 15,
        ],
        backgroundColor: [
          tailwindConfig.theme.colors.violet[500],
          tailwindConfig.theme.colors.sky[500],
          tailwindConfig.theme.colors.red[500],
          tailwindConfig.theme.colors.green[500],
        ],
        hoverBackgroundColor: [
          tailwindConfig.theme.colors.violet[600],
          tailwindConfig.theme.colors.sky[600],
          tailwindConfig.theme.colors.red[600],
          tailwindConfig.theme.colors.green[600],
        ],
        borderWidth: 0,
      },
    ],
  }

  return (
    <div className="flex flex-col col-span-full sm:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Sessions By Age</h2>
      </header>
      {/* Chart built with Chart.js 3 */}
      {/* Change the height attribute to adjust the chart height */}
      <DoughnutChart data={chartData} width={389} height={260} />
    </div>
  )
}
