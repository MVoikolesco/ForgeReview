import type { Metadata } from "next";
import "../styles/globals.scss";
import { AuthGate } from "../components/auth/AuthGate";
export const metadata: Metadata = {
  title: "ForgeReview Studio",
  description: "Visual workflow automation for code review",
};
export default function Layout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="pt-BR">
      <body><AuthGate>{children}</AuthGate></body>
    </html>
  );
}
