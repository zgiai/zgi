'use client'

import { useState } from 'react'
import { useSelectedItems } from '@/app/selected-items-context'

export const useItemSelection = (items: any[]) => {
  const { selectedItems, setSelectedItems } = useSelectedItems()
  const [isAllSelected, setIsAllSelected] = useState<boolean>(false)

  const handleCheckboxChange = (id: number, checked: boolean) => {
    setIsAllSelected(false)
    if (checked) {
      setSelectedItems([...selectedItems, id])
    } else {
      setSelectedItems(selectedItems.filter((itemId) => itemId !== id))
    }
  }

  const handleSelectAllChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setIsAllSelected(e.target.checked)
    if (e.target.checked) {
      setSelectedItems(items.map((item) => item.id))
    } else {
      setSelectedItems([])
    }
  }

  return {
    selectedItems,
    isAllSelected,
    handleCheckboxChange,
    handleSelectAllChange,
  }
}
