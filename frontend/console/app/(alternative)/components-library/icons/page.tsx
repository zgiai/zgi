export const metadata = {
  title: 'Icons - Mosaic',
  description: 'Page description',
}

export default function IconsLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Icons</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            <div className="text-sm">If you need more icons than what we currently support in Mosaic, check out these great packs 👇</div>

            {/* Nucleo */}
            <div>
              <div className="flex items-start space-x-3 mb-6">
                <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Nucleo</h2>
                <div className="btn-xs text-xs bg-violet-500/20 text-violet-600 px-2.5 py-1 rounded-full shadow-none">Recommended</div>
              </div>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg text-center xl:text-left xl:flex xl:flex-wrap xl:justify-between xl:items-center">
                <div className="text-gray-900 dark:text-gray-100 mb-4 xl:mb-0 max-w-lg mx-auto xl:mx-0 2xl:max-w-none">99% of the icons used in Mosaic come from Nucleo; a huge library of 3K+ vector icons!</div>
                <div className="inline-flex space-x-3">
                  <a className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" href="https://nucleoapp.com/pricing" target="_blank" rel="noreferrer">Buy Nucleo</a>
                  <a className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" href="https://nucleoapp.com/free-icons" target="_blank" rel="noreferrer">Download Free Pack</a>
                </div>
              </div>
            </div>

            {/* Tabler Icons */}
            <div>
              <div className="flex items-start space-x-3 mb-6">
                <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold">Tabler Icons</h2>
                <div className="text-xs inline-flex font-medium bg-green-500/20 text-green-700 rounded-full text-center px-2.5 py-1">Free Alternative</div>
              </div>
              <div className="px-6 py-8 bg-gray-100 dark:bg-gray-800/50 rounded-lg text-center xl:text-left xl:flex xl:flex-wrap xl:justify-between xl:items-center">
                <div className="text-gray-900 dark:text-gray-100 mb-4 xl:mb-0 max-w-lg mx-auto xl:mx-0 2xl:max-w-none">A set of 1250+ icons that are visually consistent with the style used in Mosaic!</div>
                <div className="inline-flex space-x-3">
                  <a className="btn bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" href="https://tablericons.com/" target="_blank" rel="noreferrer">Download Tabler Icons</a>
                </div>
              </div>
            </div>

          </div>

        </div>

      </div>
    </div>
  )
}
