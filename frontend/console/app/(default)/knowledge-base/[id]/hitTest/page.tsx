import { Metadata } from "next";
import HitTestPage from "./testPage";

export const metadata: Metadata = {
    title: "HitTest",
    description: "HitTest",
};

export default function HitTest({ params }: { params: { id: string } }) {
    return <HitTestPage kb_id={params?.id||""} />;
}