import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { Dialog } from '@headlessui/react'
import { Switch } from '@headlessui/react'
import { Combobox } from '@headlessui/react'
import React, { useState, useEffect, useMemo } from 'react'

const ModelList = ({ providerId }: { providerId: string }) => {
  const [localQuery, setLocalQuery] = useState('')
  const [isAddingModel, setIsAddingModel] = useState(false)
  const [newModelName, setNewModelName] = useState('')

  const {
    providers,
    selectedModels,
    updateSelectModelList,
    addCustomModel,
    removeSelectModelList,
    removeCustomModel,
  } = useAppSettingsStore()

  // Combine built-in and custom models
  const allModels = useMemo(
    () => [
      ...(providers[providerId]?.models || []),
      ...(providers[providerId]?.customModels || []),
    ],
    [providers, providerId],
  )

  const filteredModels = useMemo(() => {
    return allModels.filter((model) => model.name.toLowerCase().includes(localQuery.toLowerCase()))
  }, [allModels, localQuery])

  const selectedModelsData = useMemo(() => {
    return allModels.filter((model) => selectedModels[providerId]?.includes(model.id))
  }, [allModels, localQuery, selectedModels])

  // Handle input change
  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setLocalQuery(event.target.value)
  }

  // Handle clear search
  const handleClearSearch = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setLocalQuery('')
  }

  // Handle adding new custom model
  const handleAddModel = () => {
    if (newModelName.trim()) {
      addCustomModel(providerId, {
        id: newModelName.trim(),
        name: newModelName.trim(),
        isCustom: true,
      })
      setNewModelName('')
      setIsAddingModel(false)
      selectedModels[providerId]
    }
  }

  return (
    <div className="space-y-2">
      <label className="block text-sm font-medium text-gray-700">
        模型列表
        <span className="text-gray-400 font-normal ml-2">
          选择在会话中展示的模型，选择的模型会在模型列表中展示
        </span>
      </label>
      <div className="relative">
        <Combobox
          onClose={() => setLocalQuery('')}
          as="div"
          immediate
          multiple
          onChange={(modelIds: string[]) => {
            updateSelectModelList(providerId, modelIds)
          }}
        >
          <div className="relative">
            <div className="flex flex-wrap gap-2 p-2 border rounded-md min-h-[42px]">
              {selectedModelsData?.map((model) => (
                <span
                  key={model.id}
                  className="inline-flex items-center px-2 py-1 bg-gray-100 rounded-md text-sm"
                >
                  {model.name}
                  <button
                    className="ml-1 text-gray-400 hover:text-gray-600 hover:animate-pulse-fast hover:bg-gray-200 rounded-full px-1"
                    onClick={(e) => {
                      e.stopPropagation()
                      removeSelectModelList(providerId, [model.id])
                    }}
                  >
                    ×
                  </button>
                </span>
              ))}
              <div className="flex-1 flex items-center">
                <Combobox.Input
                  className="combobox-input flex-1 outline-none min-w-[120px]"
                  placeholder="Search models"
                  value={localQuery}
                  onChange={handleInputChange}
                />
                <div className="flex items-center space-x-1">
                  {localQuery && (
                    <button
                      className="p-1 hover:bg-gray-100 rounded-full"
                      onClick={handleClearSearch}
                      title="Clear search"
                    >
                      <svg className="w-4 h-4 text-gray-400" viewBox="0 0 24 24">
                        <path
                          fill="currentColor"
                          d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"
                        />
                      </svg>
                    </button>
                  )}
                  <button
                    className="p-1 hover:bg-gray-100 rounded-full add-model-button"
                    onClick={(e) => {
                      e.stopPropagation()
                      setIsAddingModel(true)
                    }}
                    title="Add custom model"
                  >
                    <svg className="w-4 h-4 text-gray-400" viewBox="0 0 24 24">
                      <path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>

            <Combobox.Options className="absolute w-full bg-white rounded-md shadow-lg max-h-60 overflow-auto custom-thin-scrollbar border mt-1 z-[70]">
              {filteredModels.length === 0 ? (
                <div className="px-4 py-2 text-sm text-gray-500">没有找到模型</div>
              ) : (
                filteredModels.map((model) => (
                  <Combobox.Option
                    key={model.id}
                    value={model.id}
                    className={({ active }) =>
                      `relative cursor-pointer select-none py-2 pl-3 pr-9 ${
                        selectedModels[providerId]?.includes(model.id)
                          ? 'bg-blue-50 text-blue-600'
                          : 'text-gray-900'
                      }`
                    }
                  >
                    <div className="flex items-center justify-between">
                      <span className="block truncate">{model.name}</span>
                      <div className="absolute right-2 flex items-center space-x-2">
                        {selectedModels[providerId]?.includes(model.id) && (
                          <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                            <path
                              fillRule="evenodd"
                              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                              clipRule="evenodd"
                            />
                          </svg>
                        )}
                        {model.isCustom && (
                          <button
                            onClick={(e) => {
                              e.stopPropagation()
                              removeCustomModel(providerId, [model.id])
                            }}
                            className="text-gray-400 hover:text-red-500"
                          >
                            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                              <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                            </svg>
                          </button>
                        )}
                      </div>
                    </div>
                  </Combobox.Option>
                ))
              )}
            </Combobox.Options>
          </div>
        </Combobox>
      </div>

      {/* Add Custom Model Dialog */}
      {isAddingModel && (
        <div
          className="fixed inset-0 bg-black bg-opacity-25 flex items-center justify-center z-[60]"
          onClick={() => {
            setIsAddingModel(false)
            setNewModelName('')
          }}
        >
          <div className="bg-white rounded-lg p-6 w-96" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-medium mb-4">Add Custom Model</h3>
            <input
              type="text"
              className="w-full px-3 py-2 border rounded-md mb-4"
              placeholder="Enter model name"
              value={newModelName}
              onChange={(e) => setNewModelName(e.target.value)}
              autoFocus
            />
            <div className="flex justify-end space-x-2">
              <button
                className="px-4 py-2 text-gray-600 hover:bg-gray-100 rounded-md"
                onClick={() => {
                  setIsAddingModel(false)
                  setNewModelName('')
                }}
              >
                Cancel
              </button>
              <button
                className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 disabled:opacity-50"
                onClick={handleAddModel}
                disabled={!newModelName.trim()}
              >
                Add
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export default ModelList
