import type { Metadata } from "next";
import favicon from "../asset/xspace-favicon.svg";
import "./globals.css";
import "@livekit/components-styles";

export const metadata: Metadata = {
  title: "Xpace - Secure Collaboration",
  description: "Premium, secure, and adaptive video collaboration for modern teams.",
  icons: { icon: favicon.src, shortcut: favicon.src, apple: favicon.src },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="id" className="h-full antialiased">
      <body className="min-h-full flex flex-col" suppressHydrationWarning>
        {children}
      </body>
    </html>
  );
}
