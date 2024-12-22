'use client'

import { useState } from 'react'
import ModalBlank from '@/components/modal-blank'

export default function FeedbackExamples() {

  const [successModalOpen, setSuccessModalOpen] = useState<boolean>(false)
  const [dangerModalOpen, setDangerModalOpen] = useState<boolean>(false)
  const [infoModalOpen, setInfoModalOpen] = useState<boolean>(false) 

  return (
    <div>
      <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Feedback</h2>
      <div className="flex flex-wrap items-center -m-1.5">

        {/* Success Modal */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setSuccessModalOpen(true) }}>Success Modal</button>
          <ModalBlank isOpen={successModalOpen} setIsOpen={setSuccessModalOpen}>
            <div className="p-5 flex space-x-4">
              {/* Icon */}
              <div className="w-10 h-10 rounded-full flex items-center justify-center shrink-0 bg-gray-100 dark:bg-gray-700">
                <svg className="shrink-0 fill-current text-green-500" width="16" height="16" viewBox="0 0 16 16">
                  <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zM7 11.4L3.6 8 5 6.6l2 2 4-4L12.4 6 7 11.4z" />
                </svg>
              </div>
              {/* Content */}
              <div>
                {/* Modal header */}
                <div className="mb-2">
                  <div className="text-lg font-semibold text-gray-800 dark:text-gray-100">Upgrade your Subscription?</div>
                </div>
                {/* Modal content */}
                <div className="text-sm mb-10">
                  <div className="space-y-2">
                    <p>Semper eget duis at tellus at urna condimentum mattis pellentesque lacus suspendisse faucibus interdum.</p>
                  </div>
                </div>
                {/* Modal footer */}
                <div className="flex flex-wrap justify-end space-x-2">
                  <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setSuccessModalOpen(false) }}>Cancel</button>
                  <button className="btn-sm bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setSuccessModalOpen(false) }}>Yes, Upgrade it</button>
                </div>
              </div>
            </div>
          </ModalBlank>
          {/* End */}
        </div>

        {/* Danger Modal */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setDangerModalOpen(true) }}>Danger Modal</button>
          <ModalBlank isOpen={dangerModalOpen} setIsOpen={setDangerModalOpen}>
            <div className="p-5 flex space-x-4">
              {/* Icon */}
              <div className="w-10 h-10 rounded-full flex items-center justify-center shrink-0 bg-gray-100 dark:bg-gray-700">
                <svg className="shrink-0 fill-current text-red-500" width="16" height="16" viewBox="0 0 16 16">
                  <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zm0 12c-.6 0-1-.4-1-1s.4-1 1-1 1 .4 1 1-.4 1-1 1zm1-3H7V4h2v5z" />
                </svg>
              </div>
              {/* Content */}
              <div>
                {/* Modal header */}
                <div className="mb-2">
                  <div className="text-lg font-semibold text-gray-800 dark:text-gray-100">Delete 1 customer?</div>
                </div>
                {/* Modal content */}
                <div className="text-sm mb-10">
                  <div className="space-y-2">
                    <p>Semper eget duis at tellus at urna condimentum mattis pellentesque lacus suspendisse faucibus interdum.</p>
                  </div>
                </div>
                {/* Modal footer */}
                <div className="flex flex-wrap justify-end space-x-2">
                  <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setDangerModalOpen(false) }}>Cancel</button>
                  <button className="btn-sm bg-red-500 hover:bg-red-600 text-white" onClick={() => { setDangerModalOpen(false) }}>Yes, Delete it</button>
                </div>
              </div>
            </div>
          </ModalBlank>
          {/* End */}
        </div>
        
        {/* Info Modal */}
        <div className="m-1.5">
          {/* Start */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setInfoModalOpen(true) }}>Info Modal</button>
          <ModalBlank isOpen={infoModalOpen} setIsOpen={setInfoModalOpen}>
            <div className="p-5 flex space-x-4">
              {/* Icon */}
              <div className="w-10 h-10 rounded-full flex items-center justify-center shrink-0 bg-gray-100 dark:bg-gray-700">
                <svg className="shrink-0 fill-current text-violet-500" width="16" height="16" viewBox="0 0 16 16">
                  <path d="M8 0C3.6 0 0 3.6 0 8s3.6 8 8 8 8-3.6 8-8-3.6-8-8-8zm1 12H7V7h2v5zM8 6c-.6 0-1-.4-1-1s.4-1 1-1 1 .4 1 1-.4 1-1 1z" />
                </svg>
              </div>
              {/* Content */}
              <div>
                {/* Modal header */}
                <div className="mb-2">
                  <div className="text-lg font-semibold text-gray-800 dark:text-gray-100">Create new Event?</div>
                </div>
                {/* Modal content */}
                <div className="text-sm mb-10">
                  <div className="space-y-2">
                    <p>Semper eget duis at tellus at urna condimentum mattis pellentesque lacus suspendisse faucibus interdum.</p>
                  </div>
                </div>
                {/* Modal footer */}
                <div className="flex flex-wrap justify-end space-x-2">
                  <button className="btn-sm border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" onClick={() => { setInfoModalOpen(false) }}>Cancel</button>
                  <button className="btn-sm bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white" onClick={() => { setInfoModalOpen(false) }}>Yes, Create it</button>
                </div>
              </div>
            </div>
          </ModalBlank>
          {/* End */}
        </div>           

      </div>
    </div>
  )
}
