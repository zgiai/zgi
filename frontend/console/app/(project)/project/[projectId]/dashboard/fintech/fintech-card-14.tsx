'use client'

import LineChart09 from '@/components/charts/line-chart-09'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

export default function FintechCard14() {

  interface MiniChart {
    data: number[]
    growth: boolean
  }

  interface ChartData {
    labels: string[]
    datasets: {
      data: number[]
      borderColor: string
      borderWidth: number
      tension: number
      pointRadius: number
      pointHoverRadius: number
      pointBackgroundColor: string
      clip: number
    }[]
  }  

  const miniCharts: MiniChart[] = [
    // Twitter
    {
      data: [
        540, 466, 540, 466, 385, 432, 334,
        334, 289, 289, 200, 289, 222, 289,
        289, 403, 554, 304, 289, 270, 134,
        270, 829, 644, 688, 664,
      ],
      growth: true,
    },
    // Facebook
    {
      data: [
        245, 288, 332, 404, 404, 314, 314,
        314, 314, 314, 234, 314, 234, 234,
        314, 314, 314, 388, 314, 202, 202,
        202, 202, 514, 720, 642,
      ],
      growth: true,
    },
    // Google
    {
      data: [
        732, 610, 610, 504, 504, 504, 349,
        349, 504, 342, 504, 610, 391, 192,
        154, 273, 191, 191, 126, 263, 349,
        252, 323, 322, 270, 232,
      ],
      growth: false,
    },
    // Apple
    {
      data: [
        222, 222, 226, 271, 365, 365, 238,
        324, 288, 206, 324, 324, 500, 409,
        409, 273, 232, 273, 500, 570, 767,
        808, 685, 767, 685, 685,
      ],
      growth: true,
    },
    // Coinbase
    {
      data: [
        632, 510, 610, 404, 504, 404, 449,
        349, 404, 542, 404, 410, 491, 392,
        254, 273, 291, 191, 226, 363, 449,
        252, 223, 222, 170, 132,
      ],
      growth: false,
    },
  ]

  const chartData: ChartData[] = []
  
  const buildChartData = (chart: MiniChart): ChartData => {
    var obj = {
      labels: [
        '12-01-2022', '01-01-2023', '02-01-2023',
        '03-01-2023', '04-01-2023', '05-01-2023',
        '06-01-2023', '07-01-2023', '08-01-2023',
        '09-01-2023', '10-01-2023', '11-01-2023',
        '12-01-2023', '01-01-2024', '02-01-2024',
        '03-01-2024', '04-01-2024', '05-01-2024',
        '06-01-2024', '07-01-2024', '08-01-2024',
        '09-01-2024', '10-01-2024', '11-01-2024',
        '12-01-2024', '01-01-2025',
      ],
      datasets: [
        // Line
        {
          data: chart.data,
          borderColor: chart.growth ? tailwindConfig.theme.colors.green[500] : tailwindConfig.theme.colors.red[500],
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 3,
          pointBackgroundColor: chart.growth ? tailwindConfig.theme.colors.green[500] : tailwindConfig.theme.colors.red[500],
          pointHoverBackgroundColor: chart.growth ? tailwindConfig.theme.colors.green[500] : tailwindConfig.theme.colors.red[500],
          pointBorderWidth: 0,
          pointHoverBorderWidth: 0,            
          clip: 20,
          tension: 0.2,
        },
      ],
    }
    return obj
  }

  miniCharts.map((miniChart) => {
    chartData.push(buildChartData(miniChart))
  })  

  return (
    <div className="col-span-full bg-white dark:bg-gray-800 shadow-sm rounded-xl">
      <header className="px-5 py-4 border-b border-gray-100 dark:border-gray-700/60">
        <h2 className="font-semibold text-gray-800 dark:text-gray-100">Market Trends</h2>
      </header>
      <div className="p-3">
        {/* Table */}
        <div className="overflow-x-auto">
          <table className="table-auto w-full dark:text-gray-300">
            {/* Table header */}
            <thead className="text-xs uppercase text-gray-400 dark:text-gray-500 bg-gray-50 dark:bg-gray-700 dark:bg-opacity-50 rounded-sm">
              <tr>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Market</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Mkt Cap</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Chart</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Price</div>
                </th>
                <th className="p-2 whitespace-nowrap">
                  <div className="font-semibold text-left">Chg. (24h)</div>
                </th>
              </tr>
            </thead>
            {/* Table body */}
            <tbody className="text-sm font-medium divide-y divide-gray-100 dark:divide-gray-700/60">
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap md:w-1/2">
                  <div className="flex items-center">
                    <svg className="shrink-0 mr-2 sm:mr-3" width="36" height="36" viewBox="0 0 36 36">
                      <circle fill="#1F2937" cx="18" cy="18" r="18" />
                      <path fill="#12C57C" d="M18 10h-.189v2.926h.19c1.355 0 2.63.527 3.587 1.486a5.08 5.08 0 0 1 1.273 2.125h-5.05v2.926h8.057l.026-.159c.13-.79.14-1.593.033-2.389h.002l-.013-.085-.022-.134c0-.007-.002-.013-.003-.02l-.023-.139h-.002A8 8 0 0 0 18 10Zm-4.867 6.537h-3l-.026.16c-.071.43-.107.87-.107 1.303 0 4.411 3.589 8 8 8h.19v-2.926H18a5.04 5.04 0 0 1-3.588-1.486A5.041 5.041 0 0 1 12.926 18c0-.415.05-.828.149-1.228l.058-.235Z" />
                    </svg>
                    <div>
                      <div className="text-gray-800 dark:text-gray-100 uppercase">Gvolt</div>
                      <div className="text-xs text-gray-500">Greenvolt Energias</div>
                    </div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-normal text-left">33.94B</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  {/* Chart built with Chart.js 3 */}
                  <div className="w-[96px]">
                    {/* Change the height attribute to adjust the chart height */}
                    <LineChart09 data={chartData[0]} width={96} height={32} />
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left">$43.07</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left text-green-500">+$4.20 (4%)</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap md:w-1/2">
                  <div className="flex items-center">
                    <svg className="shrink-0 mr-2 sm:mr-3" width="36" height="36" viewBox="0 0 36 36">
                      <circle fill="#1877F2" cx="18" cy="18" r="18" />
                      <path
                        d="M16.023 26 16 19h-3v-3h3v-2c0-2.7 1.672-4 4.08-4 1.153 0 2.144.086 2.433.124v2.821h-1.67c-1.31 0-1.563.623-1.563 1.536V16H23l-1 3h-2.72v7h-3.257Z"
                        fill="#FFF"
                        fillRule="nonzero"
                      />
                    </svg>
                    <div>
                      <div className="text-gray-800 dark:text-gray-100 uppercase">Fb</div>
                      <div className="text-xs text-gray-500">Meta Inc.</div>
                    </div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-normal text-left">903.71B</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  {/* Chart built with Chart.js 3 */}
                  <div className="w-[96px]">
                    {/* Change the height attribute to adjust the chart height */}
                    <LineChart09 data={chartData[1]} width={96} height={32} />
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left">$324.81</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left text-green-500">+$12.20 (3.7%)</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap md:w-1/2">
                  <div className="flex items-center">
                    <svg className="shrink-0 mr-2 sm:mr-3" width="36" height="36" viewBox="0 0 36 36">
                      <circle fill="#EA4335" cx="18" cy="18" r="18" />
                      <path
                        d="M18 17v2.4h4.1c-.2 1-1.2 3-4 3-2.4 0-4.3-2-4.3-4.4 0-2.4 2-4.4 4.3-4.4 1.4 0 2.3.6 2.8 1.1l1.9-1.8C21.6 11.7 20 11 18.1 11c-3.9 0-7 3.1-7 7s3.1 7 7 7c4 0 6.7-2.8 6.7-6.8 0-.5 0-.8-.1-1.2H18Z"
                        fill="#FFF"
                        fillRule="nonzero"
                      />
                    </svg>
                    <div>
                      <div className="text-gray-800 dark:text-gray-100 uppercase">Googl</div>
                      <div className="text-xs text-gray-500">Alphabet Inc.</div>
                    </div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-normal text-left">1.70T</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  {/* Chart built with Chart.js 3 */}
                  <div className="w-[96px]">
                    {/* Change the height attribute to adjust the chart height */}
                    <LineChart09 data={chartData[2]} width={96} height={32} />
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left">$2,860.96</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left text-red-500">-$12.20 (3.7%)</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap md:w-1/2">
                  <div className="flex items-center">
                    <svg className="shrink-0 mr-2 sm:mr-3" width="36" height="36" viewBox="0 0 36 36">
                      <circle fill="#1F2937" cx="18" cy="18" r="18" />
                      <path
                        d="M18.725 10.48C19.853 9.006 21.422 9 21.422 9s.233 1.384-.888 2.717c-1.197 1.424-2.557 1.191-2.557 1.191s-.256-1.12.748-2.429Zm-.605 3.398c.58 0 1.658-.79 3.06-.79 2.414 0 3.364 1.7 3.364 1.7s-1.858.94-1.858 3.221c0 2.573 2.314 3.46 2.314 3.46s-1.617 4.506-3.802 4.506c-1.003 0-1.783-.67-2.84-.67-1.078 0-2.147.695-2.843.695C13.52 26 11 21.725 11 18.29c0-3.381 2.133-5.155 4.134-5.155 1.3 0 2.31.743 2.986.743Z"
                        fill="#FFF"
                        fillOpacity=".9"
                        fillRule="nonzero"
                      />
                    </svg>
                    <div>
                      <div className="text-gray-800 dark:text-gray-100 uppercase">Aapl</div>
                      <div className="text-xs text-gray-500">Apple Inc.</div>
                    </div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-normal text-left">2.77T</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  {/* Chart built with Chart.js 3 */}
                  <div className="w-[96px]">
                    {/* Change the height attribute to adjust the chart height */}
                    <LineChart09 data={chartData[3]} width={96} height={32} />
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left">$168.55</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left text-green-500">+$7.44 (1.4%)</div>
                </td>
              </tr>
              {/* Row */}
              <tr>
                <td className="p-2 whitespace-nowrap md:w-1/2">
                  <div className="flex items-center">
                    <svg className="shrink-0 mr-2 sm:mr-3" width="36" height="36" viewBox="0 0 36 36">
                      <circle fill="#3B7AFF" cx="18" cy="18" r="18" />
                      <path
                        d="M18.12 13.894c1.673 0 3.002 1.016 3.506 2.528H25C24.389 13.19 21.679 11 18.146 11 14.135 11 11 13.998 11 18.013S14.055 25 18.146 25c3.454 0 6.216-2.19 6.828-5.449h-3.348c-.478 1.512-1.807 2.555-3.48 2.555-2.312 0-3.932-1.747-3.932-4.093 0-2.372 1.595-4.12 3.905-4.12Z"
                        fill="#FFF"
                        fillRule="nonzero"
                      />
                    </svg>
                    <div>
                      <div className="text-gray-800 dark:text-gray-100 uppercase">Coin</div>
                      <div className="text-xs text-gray-500">Coinbase Global Inc.</div>
                    </div>
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="font-normal text-left">50.89B</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  {/* Chart built with Chart.js 3 */}
                  <div className="w-[96px]">
                    {/* Change the height attribute to adjust the chart height */}
                    <LineChart09 data={chartData[4]} width={96} height={32} />
                  </div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left">$236.48</div>
                </td>
                <td className="p-2 whitespace-nowrap">
                  <div className="text-left text-red-500">-$24,30 (6.2%)</div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
