'use client'

import PolarChart from '@/components/charts/polar-chart'

// Import utilities
import { tailwindConfig, hexToRGB } from '@/components/utils/utils'

export default function AnalyticsCard10() {

  const chartData = {
    labels: ['Males', 'Females', 'Unknown'],
    datasets: [
      {
        label: 'Sessions By Gender',
        data: [
          500, 326, 242,
        ],
        backgroundColor: [
          `rgba(${hexToRGB(tailwindConfig.theme.colors.violet[500])}, 0.8)`,
          `rgba(${hexToRGB(tailwindConfig.theme.colors.sky[500])}, 0.8)`,
          `rgba(${hexToRGB(tailwindConfig.theme.colors.green[500])}, 0.8)`,
        ],
        hoverBackgroundColor: [
          `rgba(${hexToRGB(tailwindConfig.theme.colors.violet[600])}, 0.8)`,
          `rgba(${hexToRGB(tailwindConfig.theme.colors.sky[600])}, 0.8)`,
          `rgba(${hexToRGB(tailwindConfig.theme.colors.green[600])}, 0.8)`,
        ],
        borderWidth: 0,
      },
    ],
  }

  return (
    <div className="flex flex-col col-span-full sm:col-span-6 xl:col-span-4 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Sessions By Gender</h2>
      </header>
      {/* Chart built with Chart.js 3 */}
      {/* Change the height attribute to adjust the chart height */}
      <PolarChart data={chartData} width={389} height={260} />
    </div>
  )
}
