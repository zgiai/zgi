import { Metadata } from "next";
import SettingPage from "./settingPage";

export const metadata: Metadata = {
    title: "Setting",
    description: "Setting",
}   

export default function Page({ params }: { params: { id: string } }) {
    return <SettingPage id={params.id} />
}