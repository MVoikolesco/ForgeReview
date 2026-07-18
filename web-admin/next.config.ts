import type { NextConfig } from "next";

const useStaticExport = process.env.NEXT_OUTPUT === "export";
const apiInternalUrl = process.env.API_INTERNAL_URL || "http://localhost:8088";

const nextConfig: NextConfig = {
  images: { unoptimized: true },
  poweredByHeader: false,
  ...(useStaticExport ? { output: "export" as const } : {}),
  ...(!useStaticExport
    ? {
        async rewrites() {
          return [{ source: "/api/:path*", destination: `${apiInternalUrl}/api/:path*` }];
        },
      }
    : {}),
};

export default nextConfig;
