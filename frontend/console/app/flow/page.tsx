'use client'

import { useEffect } from 'react'
import FlowPageWidgets from './FlowPage'
import ContextWrapper from '@/contexts'

export default function FlowPage() {
  useEffect(() => {
    console.log('FlowPage')
  }, [])

  return <div>
    <ContextWrapper>
      <FlowPageWidgets />
    </ContextWrapper>
  </div>
}
