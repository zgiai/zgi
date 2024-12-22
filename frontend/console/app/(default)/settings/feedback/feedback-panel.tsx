export default function FeedbackPanel() {
  return (
    <div className="grow">

      {/* Panel body */}
      <div className="p-6 space-y-6">
        <div>
          <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-4">Give Feedback</h2>
          <div className="text-sm">Our product depends on customer feedback to improve the overall experience!</div>
        </div>

        {/* Rate */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-6">How likely would you recommend us to a friend or colleague?</h3>
          <div className="w-full max-w-xl">
            <div className="relative">
              <div className="absolute left-0 top-1/2 -mt-px w-full h-0.5 bg-gray-200 dark:bg-gray-700/60" aria-hidden="true"></div>
              <ul className="relative flex justify-between w-full">
                <li className="flex">
                  <button className="w-3 h-3 rounded-full bg-white dark:bg-gray-800 border-2 border-gray-400 dark:border-gray-500">
                    <span className="sr-only">1</span>
                  </button>
                </li>
                <li className="flex">
                  <button className="w-3 h-3 rounded-full bg-white dark:bg-gray-800 border-2 border-gray-400 dark:border-gray-500">
                    <span className="sr-only">2</span>
                  </button>
                </li>
                <li className="flex">
                  <button className="w-3 h-3 rounded-full bg-violet-500 border-2 border-violet-500">
                    <span className="sr-only">3</span>
                  </button>
                </li>
                <li className="flex">
                  <button className="w-3 h-3 rounded-full bg-white dark:bg-gray-800 border-2 border-gray-400 dark:border-gray-500">
                    <span className="sr-only">4</span>
                  </button>
                </li>
                <li className="flex">
                  <button className="w-3 h-3 rounded-full bg-white dark:bg-gray-800 border-2 border-gray-400 dark:border-gray-500">
                    <span className="sr-only">5</span>
                  </button>
                </li>
              </ul>
            </div>
            <div className="w-full flex justify-between text-sm text-gray-500 dark:text-gray-400 italic mt-3">
              <div>Not at all</div>
              <div>Extremely likely</div>
            </div>
          </div>
        </section>

        {/* Tell us in words */}
        <section>
          <h3 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-5">Tell us in words</h3>
          {/* Form */}
          <label className="sr-only" htmlFor="feedback">Leave a feedback</label>
          <textarea id="feedback" className="form-textarea w-full focus:border-gray-300" rows={4} placeholder="I really enjoy…"></textarea>
        </section>
      </div>

      {/* Panel footer */}
      <footer>
        <div className="flex flex-col px-6 py-5 border-t border-gray-200 dark:border-gray-700/60">
          <div className="flex self-end">
            <button className="btn dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">Cancel</button>
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white ml-3">Save Changes</button>
          </div>
        </div>
      </footer>

    </div>
  )
}