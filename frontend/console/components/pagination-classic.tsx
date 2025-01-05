export default function PaginationClassic({ current, total, pageSize, onChange = () => { } }: { current: number, total: number, pageSize: number, onChange?: (page: number) => void }) {
  // 计算当前显示范围
  const start = (current - 1) * pageSize + 1;
  const end = Math.min(current * pageSize, total);
  
  return (
    <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between">
      <nav className="mb-4 sm:mb-0 sm:order-1" role="navigation" aria-label="Navigation">
        <ul className="flex justify-center">
          <li className="ml-3 first:ml-0">
            <button
              onClick={() => current > 1 && onChange(current - 1)}
              className={`btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 ${
                current <= 1 
                  ? 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
                  : 'text-gray-800 dark:text-gray-300 hover:border-gray-300 dark:hover:border-gray-600'
              }`}
              disabled={current <= 1}
            >
              &lt;- Previous
            </button>
          </li>
          <li className="ml-3 first:ml-0">
            <button
              onClick={() => current < Math.ceil(total / pageSize) && onChange(current + 1)}
              className={`btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 ${
                current >= Math.ceil(total / pageSize)
                  ? 'text-gray-300 dark:text-gray-600 cursor-not-allowed'
                  : 'text-gray-800 dark:text-gray-300 hover:border-gray-300 dark:hover:border-gray-600'
              }`}
              disabled={current >= Math.ceil(total / pageSize)}
            >
              Next -&gt;
            </button>
          </li>
        </ul>
      </nav>
      <div className="text-sm text-gray-500 text-center sm:text-left">
        Showing <span className="font-medium text-gray-600 dark:text-gray-300">{start}</span> to <span className="font-medium text-gray-600 dark:text-gray-300">{end}</span> of <span className="font-medium text-gray-600 dark:text-gray-300">{total}</span> results
      </div>
    </div>
  )
}