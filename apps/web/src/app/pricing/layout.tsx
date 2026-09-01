import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Plans & Pricing",
  description: "Choose an Xspace plan for secure meetings, chat, files, recordings, and enterprise collaboration.",
  alternates: { canonical: "/pricing" },
  openGraph: {
    url: "/pricing",
    title: "Xspace Plans & Pricing",
    description: "Secure collaboration plans for growing teams and enterprises.",
  },
};

export default function PricingLayout({ children }: { children: ReactNode }) {
  return children;
}
