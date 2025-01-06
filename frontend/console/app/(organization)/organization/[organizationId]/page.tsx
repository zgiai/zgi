import { redirect } from 'next/navigation'

export default function OrganizationPage({ params }: { params: { organizationId: string } }) {
    redirect(`/organization/${params.organizationId}/projects`)
}