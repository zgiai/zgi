'use client'

import { useState } from 'react'
import ModalBasic from '@/components/modal-basic'
import ModalCookies from '@/components/modal-cookies'

export default function BasicExamples() {

  const [basicModalOpen, setBasicModalOpen] = useState<boolean>(false)
  const [scrollbarModalOpen, setScrollbarModalOpen] = useState<boolean>(false)
  const [cookiesModalOpen, setCookiesModalOpen] = useState<boolean>(false)  

  return (
    <div>
      <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Basic</h2>
      <div className="flex flex-wrap items-center -m-1.5">

        {/* Basic Modal */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setBasicModalOpen(true) }}>Basic Modal</button>
          <ModalBasic isOpen={basicModalOpen} setIsOpen={setBasicModalOpen} title="Basic Modal">
            {/* Modal content */}
            <div className="px-5 pt-4 pb-1">
              <div className="text-sm">
                <div className="font-medium text-gray-800 dark:text-gray-100 mb-2">Let's Talk Paragraph</div>
                <div className="space-y-2">
                  <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
                  <p>Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
                </div>
              </div>
            </div>
            {/* Modal footer */}
            <div className="px-5 py-4">
              <div className="flex flex-wrap justify-end space-x-2">
                <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setBasicModalOpen(false) }}>Close</button>
                <button className="btn-sm bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">I Understand</button>
              </div>
            </div>
          </ModalBasic>
          {/* End */}
        </div>

        {/* Modal w/ Scroll Bar */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setScrollbarModalOpen(true) }}>Modal w/ Scroll Bar</button>
          <ModalBasic isOpen={scrollbarModalOpen} setIsOpen={setScrollbarModalOpen} title="Modal w/ Scroll Bar">
            {/* Modal content */}
            <div className="px-5 py-4">
              <div className="text-sm">
                <div className="font-medium text-gray-800 dark:text-gray-100 mb-2">Let's Talk Paragraph</div>
                <div className="space-y-2">
                  <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
                  <p>Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
                  <p>Ullamcorper malesuada proin libero nunc consequat interdum varius sit. Nec sagittis aliquam malesuada bibendum arcu vitae elementum curabitur vitae. Mattis enim ut tellus elementum el fringilla est ullamcorper eget nulla. Enim eu turpis egestas pretium aenean pharetra magna. Aliquam id diam maecenas ultricies mi eget us mauris vitae ultricies leo integer t malesuada fames ac turpis egestas maecenas pharetra volutpat lacus laoreet non.</p>
                  <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
                  <p>Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
                  <p>Ullamcorper malesuada proin libero nunc consequat interdum varius sit. Nec sagittis aliquam malesuada bibendum arcu vitae elementum curabitur vitae. Mattis enim ut tellus elementum el fringilla est ullamcorper eget nulla. Enim eu turpis egestas pretium aenean pharetra magna. Aliquam id diam maecenas ultricies mi eget us mauris vitae ultricies leo integer t malesuada fames ac turpis egestas maecenas pharetra volutpat lacus laoreet non.</p>
                </div>
              </div>
            </div>
            {/* Modal footer */}
            <div className="sticky bottom-0 px-5 py-4 bg-white dark:bg-gray-800 border-t border-gray-200 dark:border-gray-700/60">
              <div className="flex flex-wrap justify-end space-x-2">
                <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setScrollbarModalOpen(false) }}>Close</button>
                <button className="btn-sm bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">I Understand</button>
              </div>
            </div>
          </ModalBasic>
          {/* End */}
        </div>        
        

        {/* Cookies */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" aria-controls="cookies-modal" onClick={() => { setCookiesModalOpen(true) }}>Cookies</button>
          <ModalCookies isOpen={cookiesModalOpen} setIsOpen={setCookiesModalOpen} title="We use cookies 🍪">
            {/* Modal content */}
            <div className="text-sm mb-5">
              <div className="space-y-2">
                <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
                <p>Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</p>
              </div>
            </div>
            {/* Modal footer */}
            <div className="flex flex-wrap justify-end space-x-2">
              <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setCookiesModalOpen(false) }}>Decline</button>
              <button className="btn-sm bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setCookiesModalOpen(false) }}>I Accept</button>
            </div>
          </ModalCookies>
          {/* End */}
        </div>  

      </div>
    </div>
  )
}
