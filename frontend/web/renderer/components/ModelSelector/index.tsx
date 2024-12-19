import { useChatStore } from '@/store/chatStore'
import { Dialog } from '@headlessui/react'
import React, { useState } from 'react'

const ModelSelector = () => {
  const { selectedModel, setSelectedModel, models } = useChatStore()
  const [isOpen, setIsOpen] = useState(false)
  const [tempSelectedModel, setTempSelectedModel] = useState(selectedModel)

  const closeModal = () => {
    setIsOpen(false)
  }

  const openModal = () => {
    setIsOpen(true)
  }

  const confirmSelection = () => {
    setSelectedModel(tempSelectedModel)
    closeModal()
  }

  return (
    <div className="relative inline-block">
      <button
        onClick={openModal}
        className="flex items-center justify-between border border-gray-300 rounded p-2 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        <span className="truncate text-left w-32 overflow-hidden whitespace-nowrap text-ellipsis">
          {models.find((model) => model.id === selectedModel)?.name || 'Select Model'}
        </span>
        <span className="ml-2">▼</span>
      </button>

      <Dialog open={isOpen} onClose={() => {}} className="relative z-10">
        <div className="fixed inset-0 bg-black opacity-30" aria-hidden="true" />
        <div className="fixed inset-0 flex items-center justify-center p-4">
          <Dialog.Panel className="mx-auto w-4/5 rounded bg-white p-6">
            <Dialog.Title className="text-lg font-medium">Select Model</Dialog.Title>
            <div className="flex flex-col mt-4">
              {models.map((model) => (
                <button
                  key={model.id}
                  onClick={() => setTempSelectedModel(model.id)}
                  className={`flex items-center justify-between w-full px-4 py-2 text-left text-sm ${
                    tempSelectedModel === model.id ? 'bg-blue-100' : 'hover:bg-gray-100'
                  }`}
                >
                  <span className="flex items-center overflow-hidden whitespace-nowrap text-ellipsis">
                    {tempSelectedModel === model.id && (
                      <span className="mr-2 text-blue-500">✔️</span>
                    )}
                    <span className="overflow-hidden whitespace-nowrap text-ellipsis">
                      {model.name}
                    </span>
                  </span>
                  <span className="text-white whitespace-nowrap space-x-2">
                    <span className="bg-blue-300 rounded-xl px-2 py-0.5">{model.usage}</span>
                    <span
                      className="rounded-xl px-2 py-0.5"
                      style={{
                        backgroundColor: model.type !== 'free' ? '#3162FF' : '#0AB268',
                      }}
                    >
                      {model.type}
                    </span>
                  </span>
                </button>
              ))}
            </div>
            <div className="mt-4 flex justify-end space-x-2">
              <button onClick={closeModal} className="bg-gray-300 text-black rounded py-2 px-4">
                Cancel
              </button>
              <button
                onClick={confirmSelection}
                className="bg-blue-500 text-white rounded py-2 px-4"
              >
                Confirm
              </button>
            </div>
          </Dialog.Panel>
        </div>
      </Dialog>
    </div>
  )
}

export default ModelSelector
