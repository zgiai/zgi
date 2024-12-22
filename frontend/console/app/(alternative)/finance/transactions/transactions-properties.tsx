export const TransactionsProperties = () => {
  const statusColor = (status: string): string => {
    switch (status) {
      case 'Completed':
        return 'bg-green-500/20 text-green-700';
      case 'Canceled':
        return 'bg-red-500/20 text-red-700';
      default:
        return 'bg-gray-400/20 text-gray-500 dark:text-gray-400';
    }
  }

  const amountColor = (amount: string): string => {
    switch (amount.charAt(0)) {
      case '+':
        return 'text-green-500'
      default:
        return 'text-gray-800 dark:text-gray-300'
    }
  } 

  return {
    statusColor,
    amountColor,
  }
}
