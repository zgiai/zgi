export const metadata = {
  title: 'Analytics - Mosaic',
  description: 'Page description',
}

import Datepicker from '@/components/datepicker'
import AnalyticsCard01 from './analytics-card-01'
import AnalyticsCard02 from './analytics-card-02'
import AnalyticsCard03 from './analytics-card-03'
import AnalyticsCard04 from './analytics-card-04'
import AnalyticsCard05 from './analytics-card-05'
import AnalyticsCard06 from './analytics-card-06'
import AnalyticsCard07 from './analytics-card-07'
import AnalyticsCard08 from './analytics-card-08'
import AnalyticsCard09 from './analytics-card-09'
import AnalyticsCard10 from './analytics-card-10'
import AnalyticsCard11 from './analytics-card-11'

export default function Analytics() {
  return (
    <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

      {/* Page header */}
      <div className="sm:flex sm:justify-between sm:items-center mb-8">

        {/* Left: Title */}
        <div className="mb-4 sm:mb-0">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Analytics</h1>
        </div>

        {/* Right: Actions */}
        <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">

          {/* Datepicker built with flatpickr */}
          <Datepicker align="right" />

        </div>

      </div>

      {/* Cards */}
      <div className="grid grid-cols-12 gap-6">

        {/* Line chart (Analytics) */}
        <AnalyticsCard01 />
        {/*  Line chart (Active Users Right Now) */}
        <AnalyticsCard02 />
        {/* Stacked bar chart (Acquisition Channels) */}
        <AnalyticsCard03 />
        {/* Horizontal bar chart (Audience Overview) */}
        <AnalyticsCard04 />
        {/* Report card (Top Channels) */}
        <AnalyticsCard05 />
        {/* Report card (Top Pages) */}
        <AnalyticsCard06 />
        {/* Report card (Top Countries) */}
        <AnalyticsCard07 />
        {/* Doughnut chart (Sessions By Device) */}
        <AnalyticsCard08 />
        {/* Doughnut chart (Visit By Age Category) */}
        <AnalyticsCard09 />
        {/* Polar chart (Sessions By Gender) */}
        <AnalyticsCard10 />
        {/* Table (Top Products) */}
        <AnalyticsCard11 />

      </div>
    </div>
  )
}
