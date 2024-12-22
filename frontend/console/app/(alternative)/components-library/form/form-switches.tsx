'use client'

import { useState } from 'react'

export default function FormSwitches() {

  const [toggle1, setToggle1] = useState<boolean>(true)
  const [toggle2, setToggle2] = useState<boolean>(false)
  const [toggle3, setToggle3] = useState<boolean>(false)

  return (
    <div>
      <h2 className="text-2xl text-gray-800 dark:text-gray-100 font-bold mb-6">Toggle Switch</h2>
      <div className="flex flex-wrap items-center -m-3">
        <div className="m-3 w-24">
          {/* Start */}
          <div className="flex items-center">
            <div className="form-switch">
              <input type="checkbox" id="switch-1" className="sr-only" checked={toggle1} onChange={() => setToggle1(!toggle1)} />
              <label className="bg-gray-400 dark:bg-gray-700" htmlFor="switch-1">
                <span className="bg-white shadow-sm" aria-hidden="true"></span>
                <span className="sr-only">Switch label</span>
              </label>
            </div>
            <div className="text-sm text-gray-400 dark:text-gray-500 italic ml-2">{toggle1 ? 'On' : 'Off'}</div>
          </div>
          {/* End */}
        </div>

        <div className="m-3 w-24">
          {/* Start */}
          <div className="flex items-center">
            <div className="form-switch">
              <input type="checkbox" id="switch-2" className="sr-only" checked={toggle2} onChange={() => setToggle2(!toggle2)} />
              <label className="bg-gray-400 dark:bg-gray-700" htmlFor="switch-2">
                <span className="bg-white shadow-sm" aria-hidden="true"></span>
                <span className="sr-only">Switch label</span>
              </label>
            </div>
            <div className="text-sm text-gray-400 dark:text-gray-500 italic ml-2">{toggle2 ? 'On' : 'Off'}</div>
          </div>
          {/* End */}
        </div>

        <div className="m-3 w-32">
          {/* Start */}
          <div className="flex items-center">
            <div className="form-switch">
              <input type="checkbox" id="switch-3" className="sr-only" checked={toggle3} onChange={() => setToggle3(!toggle3)} disabled />
              <label className="bg-gray-400 dark:bg-gray-700" htmlFor="switch-3">
                <span className="bg-white shadow-sm" aria-hidden="true"></span>
                <span className="sr-only">Switch label</span>
              </label>
            </div>
            <div className="text-sm text-gray-400 dark:text-gray-500 italic ml-2">Disabled</div>
          </div>
          {/* End */}
        </div>
      </div>
    </div>    
  )
}
