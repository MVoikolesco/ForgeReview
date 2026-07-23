import type { Metadata } from "next";
import { AuthGate } from "../components/auth/AuthGate";
import "../styles/globals.scss";
export const metadata: Metadata = {
  title: "ForgeReview Studio",
  description: "Visual workflow automation for code review",
};
export default function Layout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="pt-BR">
      <body>
        <AuthGate>{children}</AuthGate>
      </body>
    </html>
  );
}
