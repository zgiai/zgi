export const metadata = {
  title: 'Credit Cards - Mosaic',
  description: 'Page description',
}

export default function CreditCards() {
  return (
    <div className="lg:relative lg:flex bg-white dark:bg-gray-900">

      {/* Content */}
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="sm:flex sm:justify-between sm:items-center mb-5">

          {/* Left: Title */}
          <div className="mb-4 sm:mb-0">
            <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Cards</h1>
          </div>

          {/* Add card button */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Add Card</button>
        </div>

        {/* Filters */}
        <div className="mb-5">
          <ul className="flex flex-wrap -m-1">
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-transparent shadow-sm bg-gray-900 dark:bg-gray-100 text-white dark:text-gray-800 transition">
                View All
              </button>
            </li>
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm bg-white dark:bg-gray-800 text-gray-500 dark:text-gray-400 transition">
                Physical Cards
              </button>
            </li>
            <li className="m-1">
              <button className="inline-flex items-center justify-center text-sm font-medium leading-5 rounded-full px-3 py-1 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm bg-white dark:bg-gray-800 text-gray-500 dark:text-gray-400 transition">
                Virtual Cards
              </button>
            </li>
          </ul>
        </div>

        {/* Credit cards */}
        <div className="space-y-2">

          {/* Card 1 */}
          <label className="relative block cursor-pointer text-left w-full">
            <input type="radio" name="radio-buttons" className="peer sr-only" defaultChecked />
            <div className="p-4 rounded-lg dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm transition">
              <div className="grid grid-cols-12 items-center gap-x-2">
                {/* Card */}
                <div className="col-span-6 order-1 sm:order-none sm:col-span-3 flex items-center space-x-4 lg:sidebar-expanded:col-span-6 xl:sidebar-expanded:col-span-3">
                  <svg className="shrink-0" width="32" height="24" xmlns="http://www.w3.org/2000/svg">
                    <defs>
                      <linearGradient x1="1.829%" y1="100%" x2="100%" y2="2.925%" id="c1-a">
                        <stop stopColor="#4B5563" offset="0%" />
                        <stop stopColor="#1F2937" offset="100%" />
                        <stop stopColor="#9FA1FF" offset="100%" />
                      </linearGradient>
                    </defs>
                    <g fill="none" fillRule="evenodd">
                      <rect fill="url(#c1-a)" width="32" height="24" rx="3" />
                      <ellipse fill="#E61C24" fillRule="nonzero" cx="12.522" cy="12" rx="5.565" ry="5.647" />
                      <ellipse fill="#F99F1B" fillRule="nonzero" cx="19.432" cy="12" rx="5.565" ry="5.647" />
                      <path
                        d="M15.977 7.578A5.667 5.667 0 0 0 13.867 12c0 1.724.777 3.353 2.11 4.422A5.667 5.667 0 0 0 18.087 12a5.667 5.667 0 0 0-2.11-4.422Z"
                        fill="#F26622"
                        fillRule="nonzero"
                      />
                    </g>
                  </svg>
                  <div>
                    <div className="text-sm font-medium text-gray-800 dark:text-gray-100">_Metal</div>
                    <div className="text-xs">**7328</div>
                  </div>
                </div>
                {/* Name */}
                <div className="col-span-6 order-2 sm:order-none sm:col-span-3 text-left sm:text-center lg:sidebar-expanded:hidden xl:sidebar-expanded:block">
                  <div className="text-sm font-medium text-gray-800 dark:text-gray-100 truncate">Dominik Lamakani</div>
                </div>
                {/* Card limits */}
                <div className="col-span-6 order-1 sm:order-none sm:col-span-4 text-right sm:text-center lg:sidebar-expanded:col-span-6 xl:sidebar-expanded:col-span-4">
                  <div className="text-sm">$780,00 / $20,000</div>
                </div>
                {/* Card status */}
                <div className="col-span-6 order-2 sm:order-none sm:col-span-2 text-right lg:sidebar-expanded:hidden xl:sidebar-expanded:block">
                  <div className="text-xs inline-flex font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2.5 py-1">
                    Active
                  </div>
                </div>
              </div>
            </div>
            <div
              className="absolute inset-0 border-2 border-transparent peer-checked:border-violet-400 dark:peer-checked:border-violet-500 rounded-lg pointer-events-none"
              aria-hidden="true"
            />
          </label>

          {/* Card 2 */}
          <label className="relative block cursor-pointer text-left w-full">
            <input type="radio" name="radio-buttons" className="peer sr-only" />
            <div className="p-4 rounded-lg dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 shadow-sm transition">
              <div className="grid grid-cols-12 items-center gap-x-2">
                {/* Card */}
                <div className="col-span-6 order-1 sm:order-none sm:col-span-3 flex items-center space-x-4 lg:sidebar-expanded:col-span-6 xl:sidebar-expanded:col-span-3">
                  <svg className="shrink-0" width="32" height="24" xmlns="http://www.w3.org/2000/svg">
                    <defs>
                      <linearGradient x1="1.829%" y1="100%" x2="100%" y2="2.925%" id="c2a">
                        <stop stopColor="#4634B1" offset="0%" />
                        <stop stopColor="#9FA1FF" offset="100%" />
                        <stop stopColor="#9FA1FF" offset="100%" />
                      </linearGradient>
                    </defs>
                    <g fill="none" fillRule="evenodd">
                      <rect fill="url(#c2a)" width="32" height="24" rx="3" />
                      <ellipse fill="#E61C24" fillRule="nonzero" cx="12.522" cy="12" rx="5.565" ry="5.647" />
                      <ellipse fill="#F99F1B" fillRule="nonzero" cx="19.432" cy="12" rx="5.565" ry="5.647" />
                      <path
                        d="M15.977 7.578A5.667 5.667 0 0 0 13.867 12c0 1.724.777 3.353 2.11 4.422A5.667 5.667 0 0 0 18.087 12a5.667 5.667 0 0 0-2.11-4.422Z"
                        fill="#F26622"
                        fillRule="nonzero"
                      />
                    </g>
                  </svg>
                  <div>
                    <div className="text-sm font-medium text-gray-800 dark:text-gray-100">_Virtual</div>
                    <div className="text-xs">**7377</div>
                  </div>
                </div>
                {/* Name */}
                <div className="col-span-6 order-2 sm:order-none sm:col-span-3 text-left sm:text-center lg:sidebar-expanded:hidden xl:sidebar-expanded:block">
                  <div className="text-sm font-medium text-gray-800 dark:text-gray-100 truncate">Dominik Lamakani</div>
                </div>
                {/* Card limits */}
                <div className="col-span-6 order-1 sm:order-none sm:col-span-4 text-right sm:text-center lg:sidebar-expanded:col-span-6 xl:sidebar-expanded:col-span-4">
                  <div className="text-sm">$0 / $20,000</div>
                </div>
                {/* Card status */}
                <div className="col-span-6 order-2 sm:order-none sm:col-span-2 text-right lg:sidebar-expanded:hidden xl:sidebar-expanded:block">
                  <div className="text-xs inline-flex font-medium bg-yellow-500/20 text-yellow-700 rounded-full text-center px-2.5 py-1">
                    Blocked
                  </div>
                </div>
              </div>
            </div>
            <div
              className="absolute inset-0 border-2 border-transparent peer-checked:border-violet-400 dark:peer-checked:border-violet-500 rounded-lg pointer-events-none"
              aria-hidden="true"
            />
          </label>

        </div>

      </div>

      {/* Sidebar */}
      <div>
        <div className="lg:sticky lg:top-16 bg-gradient-to-b from-gray-100 to-white dark:from-gray-800/30 dark:to-gray-900 lg:overflow-x-hidden lg:overflow-y-auto no-scrollbar lg:shrink-0 border-t lg:border-t-0 lg:border-l border-gray-200 dark:border-gray-700/60 lg:w-[390px] lg:h-[calc(100dvh-64px)]">
          <div className="py-8 px-4 lg:px-8">
            <div className="max-w-sm mx-auto lg:max-w-none">

              <div className="text-gray-800 dark:text-gray-100 font-semibold text-center mb-6">Physical Metal Card Summary</div>

              {/* Credit Card */}
              <div className="relative aspect-[7/4] bg-gradient-to-tr from-gray-900 to-gray-800 p-5 rounded-xl overflow-hidden">
                <div className="relative h-full flex flex-col justify-between">
                  {/* Logo on card */}
                  <svg width="32" height="32" viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg" xmlnsXlink="http://www.w3.org/1999/xlink">
                    <defs>
                      <linearGradient x1="50%" y1="0%" x2="50%" y2="100%" id="icon1-b">
                        <stop stopColor="#E5E7EB" offset="0%" />
                        <stop stopColor="#9CA3AF" offset="100%" />
                      </linearGradient>
                      <linearGradient x1="50%" y1="24.537%" x2="50%" y2="99.142%" id="icon1-c">
                        <stop stopColor="#374151" offset="0%" />
                        <stop stopColor="#374151" stopOpacity="0" offset="100%" />
                      </linearGradient>
                      <path id="icon1-a" d="M16 0l16 32-16-5-16 5z" />
                    </defs>
                    <g transform="rotate(90 16 16)" fill="none" fillRule="evenodd">
                      <mask id="icon1-d" fill="#fff">
                        <use xlinkHref="#icon1-a" />
                      </mask>
                      <use fill="url(#icon1-b)" xlinkHref="#icon1-a" />
                      <path fill="url(#icon1-c)" mask="url(#icon1-d)" d="M16-6h20v38H16z" />
                    </g>
                  </svg>
                  {/* Card number */}
                  <div className="flex justify-between text-lg font-bold text-gray-200 tracking-widest drop-shadow-md">
                    <span>****</span>
                    <span>****</span>
                    <span>****</span>
                    <span>7328</span>
                  </div>
                  {/* Card footer */}
                  <div className="relative flex justify-between items-center z-10 mb-0.5">
                    {/* Card expiration */}
                    <div className="text-sm font-bold text-gray-200 tracking-widest drop-shadow-md space-x-3">
                      <span>EXP 12/24</span>
                      <span>CVC ***</span>
                    </div>
                  </div>
                  {/* Mastercard logo */}
                  <svg className="absolute bottom-0 right-0" width="48" height={28} viewBox="0 0 48 28">
                    <circle fill="#F0BB33" cx="34" cy="14" r="14" fillOpacity=".8" />
                    <circle fill="#FF5656" cx="14" cy="14" r="14" fillOpacity=".8" />
                  </svg>
                </div>
              </div>

              {/* Details */}
              <div className="mt-6">
                <div className="text-sm font-semibold text-gray-800 dark:text-gray-100 mb-1">Details</div>
                <ul>
                  <li className="flex items-center justify-between py-3 border-b border-gray-200 dark:border-gray-700/60">
                    <div className="text-sm">Card Name</div>
                    <div className="text-sm font-medium text-gray-800 dark:text-gray-100 ml-2">Physical Metal Card</div>
                  </li>
                  <li className="flex items-center justify-between py-3 border-b border-gray-200 dark:border-gray-700/60">
                    <div className="text-sm">Status</div>
                    <div className="flex items-center whitespace-nowrap">
                      <div className="w-2 h-2 rounded-full bg-green-500 mr-2" />
                      <div className="text-sm font-medium text-gray-800 dark:text-gray-100">Active</div>
                    </div>
                  </li>
                </ul>
              </div>

              {/* Payment Limits */}
              <div className="mt-6">
                <div className="text-sm font-semibold text-gray-800 dark:text-gray-100 mb-4">Payment Limits</div>
                <div className="pb-4 border-b border-gray-200 dark:border-gray-700/60">
                  <div className="flex justify-between text-sm mb-2">
                    <div>Spent This Month</div>
                    <div className="italic">
                      $750,00 <span className="text-gray-400 dark:text-gray-500">/</span> $1,500.00
                    </div>
                  </div>
                  <div className="relative w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full">
                    <div className="absolute inset-0 bg-green-400 rounded-full" aria-hidden="true" style={{ width: '50%' }} />
                  </div>
                </div>
              </div>

              {/* Withdrawal Limits */}
              <div className="mt-6">
                <div className="text-sm font-semibold text-gray-800 dark:text-gray-100 mb-4">Withdrawal Limits</div>
                <div className="pb-4 border-b border-gray-200 dark:border-gray-700/60">
                  <div className="flex justify-between text-sm mb-2">
                    <div>Withdrawn This Month</div>
                    <div className="italic">
                      $100,00 <span className="text-gray-400 dark:text-gray-500">/</span> $1,500.00
                    </div>
                  </div>
                  <div className="relative w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full">
                    <div className="absolute inset-0 bg-green-400 rounded-full" aria-hidden="true" style={{ width: '7.5%' }} />
                  </div>
                </div>
              </div>

              {/* Edit / Delete */}
              <div className="flex items-center space-x-3 mt-6">
                <div className="w-1/2">
                  <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">
                    <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M11.7.3c-.4-.4-1-.4-1.4 0l-10 10c-.2.2-.3.4-.3.7v4c0 .6.4 1 1 1h4c.3 0 .5-.1.7-.3l10-10c.4-.4.4-1 0-1.4l-4-4zM4.6 14H2v-2.6l6-6L10.6 8l-6 6zM12 6.6L9.4 4 11 2.4 13.6 5 12 6.6z" />
                    </svg>
                    <span className="ml-2">Edit Card</span>
                  </button>
                </div>
                <div className="w-1/2">
                  <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-red-500">
                    <svg className="fill-current shrink-0" width="16" height="16" viewBox="0 0 16 16">
                      <path d="M14.574 5.67a13.292 13.292 0 0 1 1.298 1.842 1 1 0 0 1 0 .98C15.743 8.716 12.706 14 8 14a6.391 6.391 0 0 1-1.557-.2l1.815-1.815C10.97 11.82 13.06 9.13 13.82 8c-.163-.243-.39-.56-.669-.907l1.424-1.424ZM.294 15.706a.999.999 0 0 1-.002-1.413l2.53-2.529C1.171 10.291.197 8.615.127 8.49a.998.998 0 0 1-.002-.975C.251 7.29 3.246 2 8 2c1.331 0 2.515.431 3.548 1.038L14.293.293a.999.999 0 1 1 1.414 1.414l-14 14a.997.997 0 0 1-1.414 0ZM2.18 8a12.603 12.603 0 0 0 2.06 2.347l1.833-1.834A1.925 1.925 0 0 1 6 8a2 2 0 0 1 2-2c.178 0 .348.03.512.074l1.566-1.566C9.438 4.201 8.742 4 8 4 5.146 4 2.958 6.835 2.181 8Z" />
                    </svg>
                    <span className="ml-2">Block Card</span>
                  </button>
                </div>
              </div>

            </div>
          </div>
        </div>
      </div>

    </div>
  )
}
