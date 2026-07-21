import type { Metadata } from "next";
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
      <body>{children}</body>
    </html>
  );
}
