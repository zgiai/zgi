import { SELECT_VALUE_DECOLLATOR } from '@/constants'
import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@radix-ui/react-select'
import React from 'react'

const ModelSelector = () => {
  const { selectedModel, setSelectedModel } = useChatStore()
  const { allProvidersSelectedModels } = useAppSettingsStore()
  return (
    <Select
      onValueChange={(value) => {
        const [modelType, modelId] = value.split(SELECT_VALUE_DECOLLATOR)
        const models = allProvidersSelectedModels[modelType]
        const curModelData = models.find((item) => item.id === modelId)
        if (curModelData) setSelectedModel(curModelData)
      }}
      value={selectedModel?.id || ''}
    >
      <SelectTrigger className="flex items-center justify-between border border-gray-300 rounded p-2 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500">
        <SelectValue title={selectedModel?.id} placeholder="Select Model">
          <div className="overflow-hidden whitespace-nowrap text-ellipsis w-[150px] text-left">
            {selectedModel?.name}
          </div>
        </SelectValue>
        <SelectIcon />
      </SelectTrigger>
      <SelectContent
        className="bg-white rounded border border-gray-300 custom-thin-scrollbar p-2 max-h-[300px] overflow-y-auto shadow-lg"
        position="popper"
      >
        {Object.entries(allProvidersSelectedModels).map(([modelType, itemModels]) => (
          <div key={modelType}>
            <div className="px-2 py-2 font-bold">{modelType} Models</div>
            {itemModels.map((item) => (
              <SelectItem
                key={`${modelType}${SELECT_VALUE_DECOLLATOR}${item.id}`}
                value={`${modelType}${SELECT_VALUE_DECOLLATOR}${item.id}`}
                className={`flex items-center justify-between px-2 py-2 text-left text-sm rounded-lg hover:bg-gray-100 ${
                  selectedModel?.id === item.id ? 'bg-gray-200' : ''
                }`}
              >
                <span
                  className="flex-grow overflow-hidden whitespace-nowrap text-ellipsis min-w-[150px] max-w-[200px]"
                  title={item.name}
                >
                  {item.name}
                </span>
                {item.contextSize && (
                  <span className="text-white whitespace-nowrap space-x-2">
                    <span className="bg-blue-300 rounded-xl py-0.5 w-12 inline-block text-center">
                      {item.contextSize}
                    </span>
                  </span>
                )}
              </SelectItem>
            ))}
          </div>
        ))}
      </SelectContent>
    </Select>
  )
}

export default ModelSelector
