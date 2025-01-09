import { useAppSettingsStore } from '@/store/appSettingsStore'
import { Switch } from '@headlessui/react'
import {
  ChevronDownIcon,
  ChevronRightIcon,
  QuestionMarkCircledIcon,
  ReloadIcon,
} from '@radix-ui/react-icons'
import { CheckCircleIcon } from 'lucide-react'
import React, { useState } from 'react'
import ModelList from './ModelList'

const ProviderCard = ({ providerId }) => {
  const { expandedCards, toggleCard, providers, toggleProvider, updateProvider, checkResults } =
    useAppSettingsStore()

  const provider = providers[providerId]
  const curCheckResult = checkResults[providerId]
  const [showDetails, setShowDetails] = useState(false)
  const [loading, setLoading] = useState(false)

  const handleCheck = async () => {
    setLoading(true)
    await useAppSettingsStore.getState().checkProvider(providerId)
    setLoading(false)
  }

  return (
    <div className="border rounded-lg" key={providerId}>
      <div className="flex items-center justify-between p-4 bg-gray-50">
        <div className="flex items-center space-x-3">
          <button onClick={() => toggleCard(providerId)} className="p-1 hover:bg-gray-200 rounded">
            <svg
              className={`w-4 h-4 transform transition-transform ${
                expandedCards.includes(providerId) ? 'rotate-90' : ''
              }`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
          </button>
          <span className="font-medium">{provider?.name}</span>
        </div>
        <div className="flex items-center space-x-2">
          <button className="p-1 hover:bg-gray-200 rounded-full">
            <QuestionMarkCircledIcon />
          </button>
          <Switch
            checked={provider?.enabled}
            onChange={() => toggleProvider(providerId)}
            className={`${provider?.enabled ? 'bg-blue-600' : 'bg-gray-200'} relative inline-flex h-6 w-11 items-center rounded-full`}
          >
            <span className="sr-only">Enable {provider?.name}</span>
            <span
              className={`${provider?.enabled ? 'translate-x-6' : 'translate-x-1'} inline-block h-4 w-4 transform rounded-full bg-white transition`}
            />
          </Switch>
        </div>
      </div>

      {expandedCards.includes(providerId) && (
        <div className="p-4 space-y-4 border-t">
          {/* Provider specific fields */}
          {providerId === 'zgi' && (
            <>
              <div className="space-y-2">
                <label className="block text-sm font-medium text-gray-700">
                  API Key
                  <span className="text-gray-400 font-normal ml-2">
                    Please enter your OpenAI API Key
                  </span>
                </label>
                <input
                  value={provider.apiKey || ''}
                  onChange={(e) => updateProvider(providerId, { apiKey: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md"
                  placeholder="API Key"
                />
              </div>
              <div className="space-y-2">
                <label className="block text-sm font-medium text-gray-700">
                  API Proxy Address
                  <span className="text-gray-400 font-normal ml-2">
                    The authentication address must include http(s)://
                  </span>
                </label>
                <input
                  type="text"
                  value={provider.apiEndpoint || ''}
                  onChange={(e) => updateProvider(providerId, { apiEndpoint: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md"
                  placeholder="https://api.openai.com/v1"
                />
              </div>
            </>
          )}

          {providerId === 'ollama' && (
            <>
              <div className="space-y-2">
                <label className="block text-sm font-medium text-gray-700">
                  Ollama Service Address
                  <span className="text-gray-400 font-normal ml-2">
                    Leave blank if not specified locally
                  </span>
                </label>
                <input
                  type="text"
                  value={provider.apiEndpoint || ''}
                  onChange={(e) => updateProvider(providerId, { apiEndpoint: e.target.value })}
                  className="w-full px-3 py-2 border rounded-md"
                  placeholder="http://127.0.0.1:11434"
                />
              </div>
            </>
          )}

          {/* Model Selector */}
          <ModelList providerId={providerId} />

          {/* Connection Test */}
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <div className="text-sm font-medium text-gray-700">Connectivity Check</div>
              <div className="text-sm text-gray-500">
                Test if the API Key and proxy address are filled in correctly
              </div>
            </div>
            <button
              onClick={handleCheck}
              className={`px-2 py-1.5 text-sm rounded-md flex justify-center items-center ${loading ? 'bg-gray-200 text-gray-400 cursor-not-allowed' : 'bg-blue-600 text-white hover:bg-blue-700 transition-colors'}`}
              disabled={loading}
            >
              {loading ? (
                <>
                  <ReloadIcon className="mr-2 animate-spin" /> Check
                </>
              ) : (
                'Check'
              )}
            </button>
          </div>

          {curCheckResult?.error && (
            <div className="mt-2 p-2 border border-red-500 text-red-500 rounded">
              <div>
                {`Error requesting ${providerId} service, please troubleshoot or retry based on the following
                information`}
              </div>
              <div>
                <button
                  onClick={() => setShowDetails(!showDetails)}
                  className="text-blue-500 underline flex items-center space-x-2"
                >
                  {showDetails ? (
                    <>
                      <ChevronDownIcon />
                      <span>Hide Details</span>
                    </>
                  ) : (
                    <>
                      <ChevronRightIcon />
                      <span>Show Details</span>
                    </>
                  )}
                </button>
              </div>
              {showDetails && <div className="mt-1">{curCheckResult.error}</div>}
            </div>
          )}
          {curCheckResult && curCheckResult?.error === null && (
            <div className="mt-2 flex items-center p-2 border border-green-500 bg-green-100 text-green-700 rounded">
              <CheckCircleIcon className="h-5 w-5 mr-2" />
              <div className="font-semibold">Check Passed</div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default ProviderCard
