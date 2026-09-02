import type { NextConfig } from "next";

const isDevelopment = process.env.NODE_ENV === "development";

const nextConfig: NextConfig = {
  devIndicators: false,
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    const headers = [
      {key:"Cross-Origin-Resource-Policy",value:"same-origin"},
      {key:"Permissions-Policy",value:"camera=(self), microphone=(self), geolocation=(), payment=(), usb=()"},
      {key:"Referrer-Policy",value:"no-referrer"},
      {key:"X-Content-Type-Options",value:"nosniff"},
      {key:"X-Frame-Options",value:"DENY"},
      {key:"X-Permitted-Cross-Domain-Policies",value:"none"},
    ];
    if (!isDevelopment) headers.push({key:"Strict-Transport-Security",value:"max-age=31536000; includeSubDomains"});
    return [{source:"/:path*",headers}];
  },
  async rewrites() {
    return [{
      source: "/api/v1/:path*",
      destination: `${process.env.API_URL ?? "http://127.0.0.1:8080"}/api/v1/:path*`,
    }];
  },
};

export default nextConfig;
