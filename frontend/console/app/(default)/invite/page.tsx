import InvitePage from "./invitePage";

export const metadata = {
    title: 'Invite',
};

export default function Invite({ searchParams: { token } }: {
    searchParams: { token: string };
}) {
    return (
        <InvitePage token={token} />
    );
}