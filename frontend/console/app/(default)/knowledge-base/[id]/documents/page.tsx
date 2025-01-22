import KBPage from "./kbPage";

export const metadata = {
    title: 'Organization',
    description: 'Organization',
}

export default function knowledgeBases({ params }: { params: { id: string } }) {
    return (
        <KBPage id={params?.id || ""} />
    );
}