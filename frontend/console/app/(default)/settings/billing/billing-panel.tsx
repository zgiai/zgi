export default function BillingPanel() {
  return (
    <div className="grow">

      {/* Panel body */}
      <div className="p-6 space-y-6">
        <div>
          <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-4">Billing & Invoices</h2>
          <div className="text-sm">This workspace's Basic Plan is set to <strong className="font-medium">$34</strong> per month and will renew on <strong className="font-medium">July 9, 2024</strong>.</div>
        </div>

        {/* Billing Information */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-1">Billing Information</h3>
          <ul>
            <li className="md:flex md:justify-between md:items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">Payment Method</div>
              {/* Right */}
              <div className="text-sm text-gray-600 dark:text-gray-400 ml-4">
                <span className="mr-3">Mastercard ending 9282</span>
                <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">Edit</a>
              </div>
            </li>
            <li className="md:flex md:justify-between md:items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">Billing Interval</div>
              {/* Right */}
              <div className="text-sm text-gray-600 dark:text-gray-400 ml-4">
                <span className="mr-3">Annually</span>
                <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">Edit</a>
              </div>
            </li>
            <li className="md:flex md:justify-between md:items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">VAT/GST Number</div>
              {/* Right */}
              <div className="text-sm text-gray-600 dark:text-gray-400 ml-4">
                <span className="mr-3">UK849700927</span>
                <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">Edit</a>
              </div>
            </li>
            <li className="md:flex md:justify-between md:items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">Your Address</div>
              {/* Right */}
              <div className="text-sm text-gray-600 dark:text-gray-400 ml-4">
                <span className="mr-3">34 Savoy Street, London, UK, 24E8X</span>
                <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">Edit</a>
              </div>
            </li>
            <li className="md:flex md:justify-between md:items-center py-3 border-b border-gray-200 dark:border-gray-700/60">
              {/* Left */}
              <div className="text-sm text-gray-800 dark:text-gray-100 font-medium">Billing Address</div>
              {/* Right */}
              <div className="text-sm text-gray-600 dark:text-gray-400 ml-4">
                <span className="mr-3">hello@cruip.com</span>
                <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">Edit</a>
              </div>
            </li>
          </ul>
        </section>

        {/* Invoices */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-1">Invoices</h3>
          {/* Table */}
          <table className="table-auto w-full dark:text-gray-400">
            {/* Table header */}
            <thead className="text-xs uppercase text-gray-400 dark:text-gray-500">
              <tr className="flex flex-wrap md:table-row md:flex-no-wrap">
                <th className="w-full block md:w-auto md:table-cell py-2">
                  <div className="font-semibold text-left">Year</div>
                </th>
                <th className="w-full hidden md:w-auto md:table-cell py-2">
                  <div className="font-semibold text-left">Plan</div>
                </th>
                <th className="w-full hidden md:w-auto md:table-cell py-2">
                  <div className="font-semibold text-left">Amount</div>
                </th>
                <th className="w-full hidden md:w-auto md:table-cell py-2">
                  <div className="font-semibold text-right"></div>
                </th>
              </tr>
            </thead>
            {/* Table body */}
            <tbody className="text-sm">
              {/* Row */}
              <tr className="flex flex-wrap md:table-row md:flex-no-wrap border-b border-gray-200 dark:border-gray-700/60 py-2 md:py-0">
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left font-medium text-gray-800 dark:text-gray-100">2024</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left">Basic Plan - Annualy</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left font-medium">$349.00</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-right flex items-center md:justify-end">
                    <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">HTML</a>
                    <span className="block w-px h-4 bg-gray-200 dark:bg-gray-700 mx-2" aria-hidden="true"></span>
                    <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">PDF</a>
                  </div>
                </td>
              </tr>
              {/* Row */}
              <tr className="flex flex-wrap md:table-row md:flex-no-wrap border-b border-gray-200 dark:border-gray-700/60 py-2 md:py-0">
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left font-medium text-gray-800 dark:text-gray-100">2024</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left">Basic Plan - Annualy</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-left font-medium">$349.00</div>
                </td>
                <td className="w-full block md:w-auto md:table-cell py-0.5 md:py-2">
                  <div className="text-right flex items-center md:justify-end">
                    <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">HTML</a>
                    <span className="block w-px h-4 bg-gray-200 dark:bg-gray-700 mx-2" aria-hidden="true"></span>
                    <a className="font-medium text-violet-500 hover:text-violet-600 dark:hover:text-violet-400" href="#0">PDF</a>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>

      {/* Panel footer */}
      <footer>
        <div className="flex flex-col px-6 py-5 border-t border-gray-200 dark:border-gray-700/60">
          <div className="flex self-end">
            <button className="btn dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">Cancel</button>
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white ml-3">Save Changes</button>
          </div>
        </div>
      </footer>

    </div>
  )
}