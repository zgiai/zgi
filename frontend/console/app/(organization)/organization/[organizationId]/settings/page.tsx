import { redirect } from "next/navigation";

export default function Settings({ params }: { params: { organizationId: string } }) {
    const organizationId = params.organizationId
    redirect(`/organization/${organizationId}/settings/settings`)
}