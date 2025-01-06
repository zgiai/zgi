"use client"

export default function PaginationNumeric({ current, total, pageSize, onChange = () => { } }: { current: number, total: number, pageSize: number, onChange?: (page: number) => void }) {
  // 计算总页数
  const totalPages = Math.ceil(total / pageSize);
  
  // 生成要显示的页码数组
  const getPageNumbers = () => {
    const pages: (number | string)[] = [];
    if (totalPages <= 7) {
      // 如果总页数小于等于7，显示所有页码
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i);
      }
    } else {
      // 否则显示首页、尾页和当前页附近的页码
      if (current <= 4) {
        for (let i = 1; i <= 5; i++) pages.push(i);
        pages.push('...');
        pages.push(totalPages);
      } else if (current >= totalPages - 3) {
        pages.push(1);
        pages.push('...');
        for (let i = totalPages - 4; i <= totalPages; i++) pages.push(i);
      } else {
        pages.push(1);
        pages.push('...');
        for (let i = current - 1; i <= current + 1; i++) pages.push(i);
        pages.push('...');
        pages.push(totalPages);
      }
    }
    return pages;
  };

  return (
    <div className="flex justify-center">
      <nav className="flex" role="navigation" aria-label="Navigation">
        <div className="mr-2">
          <button
            onClick={() => current > 1 && onChange(current - 1)}
            disabled={current === 1}
            className={`inline-flex items-center justify-center rounded-lg leading-5 px-2.5 py-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 ${
              current === 1 ? 'text-gray-300 dark:text-gray-600' : 'text-violet-500 hover:bg-gray-50 dark:hover:bg-gray-900'
            }`}
          >
            <span className="sr-only">Previous</span><wbr />
            <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16">
              <path d="M9.4 13.4l1.4-1.4-4-4 4-4-1.4-1.4L4 8z" />
            </svg>
          </button>
        </div>
        
        <ul className="inline-flex text-sm font-medium -space-x-px rounded-lg shadow-sm">
          {getPageNumbers().map((page, index) => (
            <li key={index}>
              {page === '...' ? (
                <span className="inline-flex items-center justify-center leading-5 px-3.5 py-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 text-gray-400 dark:text-gray-500">
                  …
                </span>
              ) : (
                <button
                  onClick={() => typeof page === 'number' && onChange(page)}
                  className={`inline-flex items-center justify-center leading-5 px-3.5 py-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 
                    ${page === current 
                      ? 'text-violet-500' 
                      : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-900'
                    }
                    ${index === 0 ? 'rounded-l-lg' : ''} 
                    ${index === getPageNumbers().length - 1 ? 'rounded-r-lg' : ''}`
                  }
                >
                  {page}
                </button>
              )}
            </li>
          ))}
        </ul>

        <div className="ml-2">
          <button
            onClick={() => current < totalPages && onChange(current + 1)}
            disabled={current >= totalPages}
            className={`inline-flex items-center justify-center rounded-lg leading-5 px-2.5 py-2 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700/60 ${
              current >= totalPages ? 'text-gray-300 dark:text-gray-600' : 'text-violet-500 hover:bg-gray-50 dark:hover:bg-gray-900'
            }`}
          >
            <span className="sr-only">Next</span><wbr />
            <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16">
              <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
            </svg>
          </button>
        </div>
      </nav>
    </div>
  )
}