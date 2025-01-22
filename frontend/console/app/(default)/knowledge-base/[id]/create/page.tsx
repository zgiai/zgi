
import CreatePage from "./createPage";

export const metadata = {
    title: 'Create Document',
    description: 'Create Document',
}

export default function Page({ params }: { params: { id: string } }) {
    return (
        <CreatePage kb_id={params?.id || ""} />
    );
}