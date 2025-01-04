'use client'

import BarChart05 from '@/components/charts/bar-chart-05'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

export default function FintechCard03() {

  const chartData = {
    labels: [
      '12-01-2022', '01-01-2023', '02-01-2023',
      '03-01-2023', '04-01-2023', '05-01-2023'
    ],
    datasets: [
      // Indigo bars
      {
        label: 'Inflow',
        data: [800, 2600, 4000, 1200, 3200, 1700],
        backgroundColor: tailwindConfig.theme.colors.violet[500],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[600],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
      // Grey bars
      {
        label: 'Outflow',
        data: [2800, 1700, 900, 2900, 1950, 3100],
        backgroundColor: tailwindConfig.theme.colors.violet[200],
        hoverBackgroundColor: tailwindConfig.theme.colors.violet[300],
        barPercentage: 0.7,
        categoryPercentage: 0.7,
        borderRadius: 4,
      },
    ],
  };

  return(
    <div className="flex flex-col col-span-full sm:col-span-6 bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Cash Flow</h2>
      </header>
      {/* Chart built with Chart.js 3 */}
      {/* Change the height attribute to adjust the chart height */}
      <BarChart05 data={chartData} width={595} height={248} />
    </div>
  )
}
