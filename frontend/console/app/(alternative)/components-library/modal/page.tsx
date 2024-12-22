export const metadata = {
  title: 'Modal - Mosaic',
  description: 'Page description',
}

import BasicExamples from './basic-examples'
import FeedbackExamples from './feedback-examples'
import ProductExamples from './product-examples'

export default function ModalLibrary() {
  return (
    <div className="relative bg-white dark:bg-gray-900 h-full">
      <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

        {/* Page header */}
        <div className="mb-8">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Modal</h1>
        </div>

        <div>

          {/* Components */}
          <div className="space-y-8 mt-8">

            {/* Basic */}
            <BasicExamples />

            {/* Feedback */}
            <FeedbackExamples />

            {/* Product */}
            <ProductExamples />            

          </div>

        </div>

      </div>
    </div>
  )
}
