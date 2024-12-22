export const metadata = {
  title: 'Breadcrumb - Mosaic',
  description: 'Page description',
}

export default function BreadcrumbLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Breadcrumb</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            {/* With Slashes */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">With Slashes</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <div className="text-center">
                  {/* Start */}
                  <ul className="inline-flex flex-wrap text-sm font-medium">
                    <li className="after:content-['/'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Home</a>
                    </li>
                    <li className="after:content-['/'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Settings</a>
                    </li>
                    <li className="after:content-['/'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Notifications</a>
                    </li>
                  </ul>
                  {/* End */}
                </div>
              </div>
            </div>

            {/* With Dots */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">With Dots</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <div className="text-center">
                  {/* Start */}
                  <ul className="inline-flex flex-wrap text-sm font-medium">
                    <li className="after:content-['·'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Home</a>
                    </li>
                    <li className="after:content-['·'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Settings</a>
                    </li>
                    <li className="after:content-['·'] last:after:hidden after:text-gray-400 dark:after:text-gray-600 after:px-2">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Notifications</a>
                    </li>
                  </ul>
                  {/* End */}
                </div>
              </div>
            </div>

            {/* With Chevrons */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">With Chevrons</h2>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg">
                <div className="text-center">
                  {/* Start */}
                  <ul className="inline-flex flex-wrap text-sm font-medium">
                    <li className="flex items-center">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Home</a>
                      <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                      </svg>
                    </li>
                    <li className="flex items-center">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Settings</a>
                      <svg className="fill-current text-gray-400 dark:text-gray-600 mx-3" width="16" height="16" viewBox="0 0 16 16">
                        <path d="M6.6 13.4L5.2 12l4-4-4-4 1.4-1.4L12 8z" />
                      </svg>
                    </li>
                    <li className="flex items-center">
                      <a className="text-gray-500 dark:text-gray-400 hover:text-violet-500 dark:hover:text-violet-500" href="#0">Notifications</a>
                    </li>
                  </ul>
                  {/* End */}
                </div>
              </div>
            </div>

          </div>

        </div>

      </div>
    </div>
  )
}
