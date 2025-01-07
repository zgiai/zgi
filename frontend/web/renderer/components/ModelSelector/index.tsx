import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { Dialog } from '@headlessui/react'
import React, { useState } from 'react'

const ModelSelector = () => {
  const {
    selectedModel,
    setSelectedModel,
    models,
    ollamaModels,
    onRefreshModels,
    refreshModelsLoading,
  } = useChatStore()
  const { allProvidersSelectedModels } = useAppSettingsStore()
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

  const onClickRefresh = async () => {
    await onRefreshModels()
    setTempSelectedModel(selectedModel)
  }

  return (
    <div className="relative inline-block">
      <button
        onClick={openModal}
        className="flex items-center justify-between border border-gray-300 rounded p-2 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        <span className="truncate text-left w-32 overflow-hidden whitespace-nowrap text-ellipsis">
          {selectedModel?.name || 'Select Model'}
        </span>
        <span className="ml-2">▼</span>
      </button>

      <Dialog open={isOpen} onClose={() => {}} className="relative z-10">
        <div className="fixed inset-0 bg-black opacity-30" aria-hidden="true" />
        <div className="fixed inset-0 flex items-center justify-center p-4">
          <Dialog.Panel className="mx-auto w-4/5 rounded bg-white p-6">
            <div className="flex justify-between items-center">
              <Dialog.Title className="text-lg font-medium">Select Model</Dialog.Title>
              <button className="ml-4" title="Refresh models list" onClick={onClickRefresh}>
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  className={`h-4 w-4 ${refreshModelsLoading ? 'animate-spin' : ''}`}
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth="2"
                    d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H17"
                  />
                </svg>
              </button>
            </div>
            <div className="flex flex-col mt-4 overflow-y-auto" style={{ maxHeight: '70vh' }}>
              {[...models, ...ollamaModels]?.map((item) => (
                <button
                  key={item.model}
                  onClick={() => setTempSelectedModel(item)}
                  className={`flex items-center justify-between w-full px-4 py-2 text-left text-sm rounded-lg ${
                    tempSelectedModel?.model === item.model ? 'bg-blue-100' : 'hover:bg-gray-100'
                  }`}
                >
                  <span className="flex items-center overflow-hidden whitespace-nowrap text-ellipsis">
                    {/* {tempSelectedModel?.model === item.model && (
                      <span className="mr-2 text-blue-500">✔️</span>
                    )} */}
                    <span className="overflow-hidden whitespace-nowrap text-ellipsis">
                      {item.name}
                    </span>
                  </span>
                  <span className="text-white whitespace-nowrap space-x-2">
                    <span className="bg-blue-300 rounded-xl py-0.5 w-12 inline-block text-center">
                      {item.usage}
                    </span>
                    <span
                      className="rounded-xl py-0.5 w-16 inline-block text-center"
                      style={{
                        backgroundColor: !['free', 'local'].includes(item.type)
                          ? '#3162FF'
                          : '#0AB268',
                      }}
                    >
                      {item.type}
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
