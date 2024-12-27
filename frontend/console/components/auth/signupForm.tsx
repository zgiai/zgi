"use client"

import { register } from "@/services/auth";
import { useState } from "react";
import { message } from "antd";

export default function SignupForm() {
  const [formData, setFormData] = useState({
    email: "",
    username: "admin",
    password: "",
    password_again: ""
  })

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  }

  const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (formData.password !== formData.password_again) {
      message.error("Passwords do not match")
      return
    }
    const res = await register({
      email: formData.email,
      username: formData.username,
      password: formData.password,
    })
    // console.log(res)
    if (res?.status_code === 200) {
      message.success("Sigin up success")
    } else {
      message.error(res?.status_message)
    }
  }



  return <form onSubmit={handleSubmit}>
    <div className="space-y-4">
      <div>
        <label className="block text-sm font-medium mb-1" htmlFor="email">Email Address <span className="text-red-500">*</span></label>
        <input id="email" className="form-input w-full" type="email" name="email" value={formData?.email} onChange={handleChange} />
      </div>
      <div>
        <label className="block text-sm font-medium mb-1" htmlFor="name">User Name <span className="text-red-500">*</span></label>
        <input id="name" className="form-input w-full" type="text" name="username" value={formData?.username} onChange={handleChange} />
      </div>
      <div>
        <label className="block text-sm font-medium mb-1" htmlFor="password">Password</label>
        <input id="password" className="form-input w-full" type="password" autoComplete="on" name="password" value={formData?.password} onChange={handleChange} />
      </div>
      <div>
        <label className="block text-sm font-medium mb-1" htmlFor="password">Password Again</label>
        <input id="password-again" className="form-input w-full" type="password" autoComplete="on" name="password_again" value={formData?.password_again} onChange={handleChange} />
      </div>
    </div>
    <div className="flex items-center justify-between mt-6">
      <div className="mr-1">
        {/* <label className="flex items-center">
          <input type="checkbox" className="form-checkbox" />
          <span className="text-sm ml-2">Email me about product news.</span>
        </label> */}
      </div>
      <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white ml-3 whitespace-nowrap" type="submit">Sign Up</button>
    </div>
  </form>
}