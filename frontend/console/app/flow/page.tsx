'use client'

import { useEffect } from 'react'
import ContextWrapper from '@/contexts'
import FlowPageWidgets from './flowPage'
import './index.css'
import './applies.css'
import './classes.css'
import './i18n'
import { useState } from 'react'
import { QueryClient, QueryClientProvider } from "react-query";
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      refetchOnMount: false,
      retry: 0
    }
  }
})

export default function FlowPage() {
  const [mouted, setMouted] = useState(false)

  useEffect(() => {
    console.log('FlowPage')
    window.ThemeStyle = {
      bg: 'logo'
    }
    setMouted(true)
  }, [])

  return (
    <div>
      <QueryClientProvider client={queryClient}>
        <ContextWrapper>
          {
            mouted && <FlowPageWidgets />
          }
        </ContextWrapper>
      </QueryClientProvider>
    </div>
  )
}
