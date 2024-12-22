import Image from 'next/image'
import { Transaction } from './transactions-table'
import { TransactionsProperties } from './transactions-properties'
import { useFlyoutContext } from '@/app/flyout-context'
import { useTransactionDetail } from './transaction-context'

interface TransactionsTableItemProps {
  transaction: Transaction
  onCheckboxChange: (id: number, checked: boolean) => void
  isSelected: boolean
}

export default function TransactionsTableItem({ transaction, onCheckboxChange, isSelected }: TransactionsTableItemProps) {

  const { setFlyoutOpen } = useFlyoutContext()

  const { setTransaction } = useTransactionDetail()

  const {
    statusColor,
    amountColor,
  } = TransactionsProperties()  

  const handleCheckboxChange = (e: React.ChangeEvent<HTMLInputElement>) => {        
    onCheckboxChange(transaction.id, e.target.checked)
  }

  const handleTransactionClick = (e: React.MouseEvent<HTMLButtonElement, MouseEvent>) => {    
    e.stopPropagation()
    e.nativeEvent.stopImmediatePropagation()
    setFlyoutOpen(true)
    setTransaction(transaction)
  }

  return (
    <tr>
      <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap w-px">
        <div className="flex items-center">
          <label className="inline-flex">
            <span className="sr-only">Select</span>
            <input className="form-checkbox" type="checkbox" onChange={handleCheckboxChange} checked={isSelected} />
          </label>
        </div>
      </td>
      <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap md:w-1/2">
        <div className="flex items-center">
          <div className="w-9 h-9 shrink-0 mr-2 sm:mr-3">
            <button onClick={(e) => handleTransactionClick(e)} tabIndex={-1}>
              <Image className="rounded-full" src={transaction.image} width={36} height={36} alt={transaction.name} />
            </button>
          </div>
          <div className="font-medium text-gray-800 dark:text-gray-100">
            <button onClick={(e) => handleTransactionClick(e)}>{transaction.name}</button>
          </div>
        </div>
      </td>
      <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
        <div className="text-left">{transaction.date}</div>
      </td>
      <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap">
        <div className="text-left">
          <div className={`text-xs inline-flex font-medium rounded-full text-center px-2.5 py-1 ${statusColor(transaction.status)}`}>{transaction.status}</div>
        </div>
      </td>
      <td className="px-2 first:pl-5 last:pr-5 py-3 whitespace-nowrap w-px">
        <div className={`text-right font-medium ${amountColor(transaction.amount)}`}>{transaction.amount}</div>
      </td>
    </tr>
  )
}