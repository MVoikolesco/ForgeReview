import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "ForgeReview Console",
  description: "Gerencie conexões e modelos do ForgeReview.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="pt-BR">
      <body>{children}</body>
    </html>
  );
}
