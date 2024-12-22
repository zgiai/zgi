export const metadata = {
  title: 'Fintech - Mosaic',
  description: 'Page description',
}

import Datepicker from '@/components/datepicker'
import FintechIntro from './fintech-intro'
import FintechCard01 from './fintech-card-01'
import FintechCard02 from './fintech-card-02'
import FintechCard03 from './fintech-card-03'
import FintechCard04 from './fintech-card-04'
import FintechCard05 from './fintech-card-05'
import FintechCard06 from './fintech-card-06'
import FintechCard07 from './fintech-card-07'
import FintechCard08 from './fintech-card-08'
import FintechCard09 from './fintech-card-09'
import FintechCard10 from './fintech-card-10'
import FintechCard11 from './fintech-card-11'
import FintechCard12 from './fintech-card-12'
import FintechCard13 from './fintech-card-13'
import FintechCard14 from './fintech-card-14'

export default function Fintech() {
  return (
    <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

      {/* Page header */}
      <div className="sm:flex sm:justify-between sm:items-center mb-5">

        {/* Left: Title */}
        <div className="mb-4 sm:mb-0">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Fintech</h1>
        </div>

        {/* Right: Actions */}
        <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">

          {/* Datepicker built with flatpickr */}
          <Datepicker align="right" />

          {/* Add account button */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">
            <svg className="fill-current shrink-0 xs:hidden" width="16" height="16" viewBox="0 0 16 16">
              <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
            </svg>
            <span className="max-xs:sr-only">Add Account</span>
          </button>

        </div>

      </div>

      {/* Cards */}
      <div className="grid grid-cols-12 gap-6">

        {/* Page Intro */}
        <FintechIntro />
        {/* Line chart (Portfolio Returns) */}
        <FintechCard01 />
        {/* Credit Card */}
        <FintechCard02 />
        {/* Bar chart (Cash Flow) */}
        <FintechCard03 />
        {/* Horizontal bar chart (Cash Flow by Account) */}
        <FintechCard04 />
        {/* Table (Recent Expenses) */}
        <FintechCard05 />
        {/* Table (Recent Earnings) */}
        <FintechCard06 />
        {/* Line chart (Saving Goals) */}
        <FintechCard07 />
        {/* Line chart (Growth Portfolio) */}
        <FintechCard08 />
        {/* Pie chart (Portfolio Value) */}
        <FintechCard09 />
        {/* Line charts (Stock graphs) */}
        <FintechCard10 />
        <FintechCard11 />
        <FintechCard12 />
        <FintechCard13 />
        {/* Table (Market Trends) */}
        <FintechCard14 />        

      </div>
    </div>
  )
}
