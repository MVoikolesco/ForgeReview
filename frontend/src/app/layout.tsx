import "./studio.css";
import type { Metadata } from "next";
export const metadata: Metadata = { title: "ForgeReview Studio", description: "Visual workflow automation for code review" };
export default function Layout({ children }: Readonly<{ children: React.ReactNode }>) { return <html lang="pt-BR"><body>{children}</body></html>; }
