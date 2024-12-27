'use client'

import { useEffect } from 'react'
import ContextWrapper from '@/contexts'
import FlowPageWidgets from './FlowPage'
import './index.css'
import './i18n'

export default function FlowPage() {
  useEffect(() => {
    console.log('FlowPage')
  }, [])

  return (
    <div>
      <ContextWrapper>
        <FlowPageWidgets />
      </ContextWrapper>
    </div>
  )
}
