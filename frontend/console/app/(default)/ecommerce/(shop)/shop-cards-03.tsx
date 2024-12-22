import Image from 'next/image'
import AppImage09 from '@/public/images/applications-image-09.jpg'
import AppImage10 from '@/public/images/applications-image-10.jpg'
import AppImage11 from '@/public/images/applications-image-11.jpg'
import AppImage12 from '@/public/images/applications-image-12.jpg'

export default function ShopCards03() {
  return (
    <>
      {/* Card 1 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage09} width={286} height={160} alt="Application 09" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">Health and Happiness Workshop</h3>
              </header>
              {/* List */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M5 4a1 1 0 0 0 0 2h6a1 1 0 1 0 0-2H5Z" />
                    <path d="M4 0a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4V4a4 4 0 0 0-4-4H4ZM2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Z" />
                  </svg>
                  <div className="text-red-500">Fri 7 Aug 2024 23:00 CEST</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M3.886 1.137A8 8 0 1 1 .08 9.142a1 1 0 0 1 1.98-.284 6 6 0 1 0 3.314-6.256l.844.83a1 1 0 0 1-.53 1.698l-3.745.653a1 1 0 0 1-1.16-1.142L1.38.887A1 1 0 0 1 3.07.331l.817.806ZM9 7.586l1.707 1.707a1 1 0 1 1-1.414 1.414l-2-2A1 1 0 0 1 7 8V5a1 1 0 1 1 2 0v2.586Z" />
                  </svg>
                  <div>Starts at $16.24 / person</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Tickets</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 2 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage10} width={286} height={160} alt="Application 10" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">Health and Happiness Workshop</h3>
              </header>
              {/* List */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M5 4a1 1 0 0 0 0 2h6a1 1 0 1 0 0-2H5Z" />
                    <path d="M4 0a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4V4a4 4 0 0 0-4-4H4ZM2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Z" />
                  </svg>
                  <div className="text-red-500">Fri 7 Aug 2024 23:00 CEST</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M3.886 1.137A8 8 0 1 1 .08 9.142a1 1 0 0 1 1.98-.284 6 6 0 1 0 3.314-6.256l.844.83a1 1 0 0 1-.53 1.698l-3.745.653a1 1 0 0 1-1.16-1.142L1.38.887A1 1 0 0 1 3.07.331l.817.806ZM9 7.586l1.707 1.707a1 1 0 1 1-1.414 1.414l-2-2A1 1 0 0 1 7 8V5a1 1 0 1 1 2 0v2.586Z" />
                  </svg>
                  <div>Starts at $16.24 / person</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Tickets</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 3 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage11} width={286} height={160} alt="Application 11" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">Health and Happiness Workshop</h3>
              </header>
              {/* List */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M5 4a1 1 0 0 0 0 2h6a1 1 0 1 0 0-2H5Z" />
                    <path d="M4 0a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4V4a4 4 0 0 0-4-4H4ZM2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Z" />
                  </svg>
                  <div className="text-red-500">Fri 7 Aug 2024 23:00 CEST</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M3.886 1.137A8 8 0 1 1 .08 9.142a1 1 0 0 1 1.98-.284 6 6 0 1 0 3.314-6.256l.844.83a1 1 0 0 1-.53 1.698l-3.745.653a1 1 0 0 1-1.16-1.142L1.38.887A1 1 0 0 1 3.07.331l.817.806ZM9 7.586l1.707 1.707a1 1 0 1 1-1.414 1.414l-2-2A1 1 0 0 1 7 8V5a1 1 0 1 1 2 0v2.586Z" />
                  </svg>
                  <div>Starts at $16.24 / person</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Tickets</a>
            </div>
          </div>
        </div>
      </div>

      {/* Card 4 */}
      <div className="col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl overflow-hidden">
        <div className="flex flex-col h-full">
          {/* Image */}
          <div className="relative">
            <Image className="w-full" src={AppImage12} width={286} height={160} alt="Application 12" />
            {/* Like button */}
            <button className="absolute top-0 right-0 mt-4 mr-4">
              <div className="text-gray-100 bg-gray-900 bg-opacity-60 rounded-full">
                <span className="sr-only">Like</span>
                <svg className="h-8 w-8 fill-current" viewBox="0 0 32 32">
                  <path d="M22.682 11.318A4.485 4.485 0 0019.5 10a4.377 4.377 0 00-3.5 1.707A4.383 4.383 0 0012.5 10a4.5 4.5 0 00-3.182 7.682L16 24l6.682-6.318a4.5 4.5 0 000-6.364zm-1.4 4.933L16 21.247l-5.285-5A2.5 2.5 0 0112.5 12c1.437 0 2.312.681 3.5 2.625C17.187 12.681 18.062 12 19.5 12a2.5 2.5 0 011.785 4.251h-.003z" />
                </svg>
              </div>
            </button>
          </div>
          {/* Card Content */}
          <div className="grow flex flex-col p-5">
            {/* Card body */}
            <div className="grow">
              {/* Header */}
              <header className="mb-2">
                <h3 className="text-lg text-gray-800 dark:text-gray-100 font-semibold">Health and Happiness Workshop</h3>
              </header>
              {/* List */}
              <ul className="text-sm space-y-2 mb-5 dark:text-gray-300">
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M5 4a1 1 0 0 0 0 2h6a1 1 0 1 0 0-2H5Z" />
                    <path d="M4 0a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4V4a4 4 0 0 0-4-4H4ZM2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Z" />
                  </svg>
                  <div className="text-red-500">Fri 7 Aug 2024 23:00 CEST</div>
                </li>
                <li className="flex items-center">
                  <svg className="fill-current text-gray-400 dark:text-gray-500 shrink-0 mr-3" width="16" height="16" viewBox="0 0 16 16">
                    <path d="M3.886 1.137A8 8 0 1 1 .08 9.142a1 1 0 0 1 1.98-.284 6 6 0 1 0 3.314-6.256l.844.83a1 1 0 0 1-.53 1.698l-3.745.653a1 1 0 0 1-1.16-1.142L1.38.887A1 1 0 0 1 3.07.331l.817.806ZM9 7.586l1.707 1.707a1 1 0 1 1-1.414 1.414l-2-2A1 1 0 0 1 7 8V5a1 1 0 1 1 2 0v2.586Z" />
                  </svg>
                  <div>Starts at $16.24 / person</div>
                </li>
              </ul>
            </div>
            {/* Card footer */}
            <div>
              <a className="btn-sm w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="#0">Buy Tickets</a>
            </div>
          </div>
        </div>
      </div>
    </>
  )
}