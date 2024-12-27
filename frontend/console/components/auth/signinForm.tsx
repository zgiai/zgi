"use client"

import { login } from "@/services/auth";
import { message } from "antd";
import Link from "next/link";
import { useState } from "react";


export default function SigininForm() {
    const [formData, setFormData] = useState({
        email: "",
        password: "",
    })

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setFormData({ ...formData, [e.target.name]: e.target.value });
    }

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault()
        const res = await login({
            email: formData.email,
            password: formData.password,
        })
        // console.log(res)
        if (res?.status_code === 200) {
            message.success("Sigin in success")
            localStorage.setItem("token", res?.data?.access_token || "")
            // console.log(res)
            location.href = "/"
        } else {
            message.error(res?.status_message)
        }
    }


    return <form onSubmit={handleSubmit}>
        <div className="space-y-4">
            <div>
                <label className="block text-sm font-medium mb-1" htmlFor="email">Email Address</label>
                <input id="email" className="form-input w-full" type="email" value={formData?.email} name="email" onChange={handleChange} />
            </div>
            <div>
                <label className="block text-sm font-medium mb-1" htmlFor="password">Password</label>
                <input id="password" className="form-input w-full" type="password" autoComplete="on" value={formData?.password} name="password" onChange={handleChange} />
            </div>
        </div>
        <div className="flex items-center justify-between mt-6">
            <div className="mr-1">
                <Link className="text-sm underline hover:no-underline" href="/reset-password">Forgot Password?</Link>
            </div>
            <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white ml-3" type="submit" >Sign In</button>
        </div>
    </form>
}