import { Metadata } from "next";
import MemberPage from "./memberPage";

export const metadata: Metadata = {
  title: 'Member',
  description: 'Member',
};

export default function Member() {
  return <MemberPage />;
}
