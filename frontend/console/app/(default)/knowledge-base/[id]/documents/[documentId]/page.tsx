import DocumentPage from "./documentPage";
import { Metadata } from "next";

export const metadata: Metadata = {
    title: "Document Page",
    description: "Document Page",
}

export default function Page({ params }: { params: { id: string, documentId: string } }) {
    return <DocumentPage kbId={params.id} documentId={params.documentId} />;
}