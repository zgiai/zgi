import Link from 'next/link'
import Image from 'next/image'
import Sidebar from '@/components/ui/sidebar'
import Header from '@/components/ui/header'
import NotFoundImage from '@/public/images/404-illustration.svg'
import NotFoundImageDark from '@/public/images/404-illustration-dark.svg'

export default function NotFound() {
  return (
    <div className="flex h-[100dvh] overflow-hidden">

      {/* Sidebar */}
      <Sidebar />

      {/* Content area */}
      <div className="relative flex flex-col flex-1 overflow-y-auto overflow-x-hidden">

        {/*  Site header */}
        <Header />

        <main className="grow [&>*:first-child]:scroll-mt-16">
          <div className="relative bg-white dark:bg-gray-900 h-full">
            <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

              <div className="max-w-2xl m-auto mt-16">

                <div className="text-center px-4">
                  <div className="inline-flex mb-8">
                    <Image className="dark:hidden" src={NotFoundImage} width={176} height={176} alt="404 illustration" />
                    <Image className="hidden dark:block" src={NotFoundImageDark} width={176} height={176} alt="404 illustration dark" />                 
                  </div>
                  <div className="mb-6">Hmm...this page doesn't exist. Try searching for something else!</div>
                  <Link href="/" className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Back To Dashboard</Link>
                </div>

              </div>

            </div>
          </div>
        </main>        

      </div>

    </div>    
  )
}