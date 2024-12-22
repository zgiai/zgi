export const metadata = {
  title: 'Accordion - Mosaic',
  description: 'Page description',
}

import AccordionBasic from '@/components/accordion-basic'
import AccordionTableItem from '@/components/accordion-table-item'
import AccordionTableRichItem from '@/components/accordion-table-rich-item'
import Image01 from '@/public/images/user-40-07.jpg'

export default function AccordionLibrary() {

  // Some dummy data
  const items = [
    {
      id: 0,
      image: Image01,
      customer: 'Mark Cameron',
      total: '$129.00',
      status: 'Refunded',
      items: 1,
      location: '🇲🇽 New Mexico, MX',
      type: 'Subscription',
      description: 'Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.'
    },
  ]

  const items2 = [
    {
      id: 0,
      image: Image01,
      customer: 'Mark Cameron',
      email: 'mark.cameron@app.com',
      location: '🇬🇧 London, UK',
      date: '22/01/2024',
      amount: '+249.88',
      descriptionTitle: 'Excepteur sint occaecat cupidatat.',
      descriptionBody: 'Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam quis. Ut enim ad minim veniam quis. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.',
    },
  ]  

  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Accordion</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            {/* Basic Accordion */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Basic Accordion</h2>
              <AccordionBasic title="Accordion Title">
                Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam quis. Ut enim ad minim veniam quis. Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
              </AccordionBasic>
            </div>

            {/* Table Row with Accordion */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Table Row with Accordion</h2>
              {/* Start */}
              <div className="rounded-lg border border-gray-200 dark:border-gray-700/60">
                <div className="overflow-x-auto">
                  <table className="table-auto w-full dark:text-gray-300 divide-y divide-gray-100 dark:divide-gray-700/60">
                    {items.map(item => (
                      <AccordionTableItem key={item.id} item={item} />
                    ))}
                  </table>
                </div>
              </div>
              {/* End */}
            </div>

            {/* Rich Table Row with Accordion */}
            <div>
              <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Rich Table Row with Accordion</h2>
              {/* Start */}
              <div className="rounded-lg border border-gray-200 dark:border-gray-700/60">
                <div className="overflow-x-auto">
                  <table className="table-auto w-full dark:text-gray-300 divide-y divide-gray-100 dark:divide-gray-700/60">
                    {items2.map(item => (
                      <AccordionTableRichItem key={item.id} item={item} />
                    ))}
                  </table>
                </div>
              </div>
              {/* End */}
            </div>

          </div>

        </div>

      </div>
    </div>
  )
}
