import Image from 'next/image'
import AppImage17 from '@/public/images/applications-image-17.jpg'
import AppImage18 from '@/public/images/applications-image-18.jpg'
import AppImage19 from '@/public/images/applications-image-19.jpg'
import AppImage20 from '@/public/images/applications-image-20.jpg'

export default function ShopCards06() {
  return (
    <>
      {/* Card 1 */}
      <div className="relative col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl dark:border-transparent overflow-hidden">
        {/* Image */}
        <Image className="absolute w-full h-full object-cover" src={AppImage17} width={286} height={160} alt="Application 17" />
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-gray-800 to-transparent" aria-hidden="true"></div>
        {/* Content */}
        <div className="relative h-full p-5 flex flex-col justify-end">
          <h3 className="text-lg text-white font-semibold mt-16 mb-0.5">Merchandise</h3>
          <a className="text-sm font-medium text-violet-400 hover:text-violet-300" href="#0">Explore -&gt;</a>
        </div>
      </div>

      {/* Card 2 */}
      <div className="relative col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl dark:border-transparent overflow-hidden">
        {/* Image */}
        <Image className="absolute w-full h-full object-cover" src={AppImage18} width={286} height={160} alt="Application 18" />
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-gray-800 to-transparent" aria-hidden="true"></div>
        {/* Content */}
        <div className="relative h-full p-5 flex flex-col justify-end">
          <h3 className="text-lg text-white font-semibold mt-16 mb-0.5">Audiobooks</h3>
          <a className="text-sm font-medium text-violet-400 hover:text-violet-300" href="#0">Explore -&gt;</a>
        </div>
      </div>

      {/* Card 3 */}
      <div className="relative col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl dark:border-transparent overflow-hidden">
        {/* Image */}
        <Image className="absolute w-full h-full object-cover" src={AppImage19} width={286} height={160} alt="Application 19" />
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-gray-800 to-transparent" aria-hidden="true"></div>
        {/* Content */}
        <div className="relative h-full p-5 flex flex-col justify-end">
          <h3 className="text-lg text-white font-semibold mt-16 mb-0.5">Design & Tech</h3>
          <a className="text-sm font-medium text-violet-400 hover:text-violet-300" href="#0">Explore -&gt;</a>
        </div>
      </div>

      {/* Card 4 */}
      <div className="relative col-span-full sm:col-span-6 xl:col-span-3 bg-white dark:bg-gray-800 shadow-sm rounded-xl dark:border-transparent overflow-hidden">
        {/* Image */}
        <Image className="absolute w-full h-full object-cover" src={AppImage20} width={286} height={160} alt="Application 20" />
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-gray-800 to-transparent" aria-hidden="true"></div>
        {/* Content */}
        <div className="relative h-full p-5 flex flex-col justify-end">
          <h3 className="text-lg text-white font-semibold mt-16 mb-0.5">Apps & Software</h3>
          <a className="text-sm font-medium text-violet-400 hover:text-violet-300" href="#0">Explore -&gt;</a>
        </div>
      </div>
    </>
  )
}