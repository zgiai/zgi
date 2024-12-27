'use client'

import { useEffect } from 'react'
import FlowPageWidgets from './FlowPage'

export default function FlowPage() {
  useEffect(() => {
    console.log('FlowPage')
  }, [])

  return <div>FlowPage

    <FlowPageWidgets />
  </div>
}
