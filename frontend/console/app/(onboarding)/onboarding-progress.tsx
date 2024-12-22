import Link from 'next/link'

export default function OnboardingProgress({ step = 1 }: { step?: number}) {
  return (
    <div className="px-4 pt-12 pb-8">
      <div className="max-w-md mx-auto w-full">
        <div className="relative">
          <div className="absolute left-0 top-1/2 -mt-px w-full h-0.5 bg-gray-200 dark:bg-gray-700/60" aria-hidden="true"></div>
          <ul className="relative flex justify-between w-full">
            <li>
              <Link className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold ${step >= 1 ? 'bg-violet-500 text-white' : 'bg-white dark:bg-gray-900 text-gray-500 dark:text-gray-400'}`} href="/onboarding-01">1</Link>
            </li>
            <li>
              <Link className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold ${step >= 2 ? 'bg-violet-500 text-white' : 'bg-white dark:bg-gray-900 text-gray-500 dark:text-gray-400'}`} href="/onboarding-02">2</Link>
            </li>
            <li>
              <Link className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold ${step >= 3 ? 'bg-violet-500 text-white' : 'bg-white dark:bg-gray-900 text-gray-500 dark:text-gray-400'}`} href="/onboarding-03">3</Link>
            </li>
            <li>
              <Link className={`flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold ${step >= 4 ? 'bg-violet-500 text-white' : 'bg-white dark:bg-gray-900 text-gray-500 dark:text-gray-400'}`} href="/onboarding-04">4</Link>
            </li>
          </ul>
        </div>
      </div>
    </div>
  )
}
