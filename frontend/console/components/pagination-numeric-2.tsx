
export default function PaginationNumeric02() {
  return (
    <div>
      <nav className="flex justify-between" role="navigation" aria-label="Navigation">
        <div className="flex-1 mr-2">
          <span className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 text-gray-300 dark:text-gray-600">
            &lt;-<span className="hidden sm:inline">&nbsp;Previous</span>
          </span>
        </div>
        <div className="grow text-center">
          <ul className="inline-flex text-sm font-medium -space-x-px">
            <li>
              <span className="inline-flex items-center justify-center rounded-full leading-5 px-2 py-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 text-violet-500 shadow-sm">
                <span className="w-5">1</span>
              </span>
            </li>
            <li>
              <a
                className="inline-flex items-center justify-center leading-5 px-2 py-2 text-gray-600 dark:text-gray-300 hover:text-violet-500 dark:hover:text-violet-500 border border-transparent"
                href="#0"
              >
                <span className="w-5">2</span>
              </a>
            </li>
            <li>
              <a
                className="inline-flex items-center justify-center leading-5 px-2 py-2 text-gray-600 dark:text-gray-300 hover:text-violet-500 dark:hover:text-violet-500 border border-transparent"
                href="#0"
              >
                <span className="w-5">3</span>
              </a>
            </li>
            <li>
              <span className="inline-flex items-center justify-center leading-5 px-2 py-2 text-gray-400 dark:text-gray-500">…</span>
            </li>
            <li>
              <a
                className="inline-flex items-center justify-center rounded-r leading-5 px-2 py-2 text-gray-600 dark:text-gray-300 hover:text-violet-500 dark:hover:text-violet-500 border border-transparent"
                href="#0"
              >
                <span className="w-5">9</span>
              </a>
            </li>
          </ul>
        </div>
        <div className="flex-1 text-right ml-2">
          <a className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" href="#0">
            <span className="hidden sm:inline">Next&nbsp;</span>-&gt;
          </a>
        </div>
      </nav>
    </div>
  );
}