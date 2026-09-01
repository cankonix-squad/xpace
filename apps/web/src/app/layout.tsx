import type { Metadata } from "next";
import favicon from "../asset/xspace-favicon.svg";
import "@livekit/components-styles";
import "./globals.css";

export const metadata: Metadata = {
  metadataBase: new URL("https://xspace.cankonix.com"),
  title: { default: "Xspace — Secure Collaboration", template: "%s · Xspace" },
  description: "Premium, secure, and adaptive video collaboration for modern teams.",
  applicationName: "Xspace",
  alternates: { canonical: "/" },
  icons: { icon: favicon.src, shortcut: favicon.src, apple: favicon.src },
  openGraph: {
    type: "website",
    locale: "id_ID",
    url: "/",
    siteName: "Xspace",
    title: "Xspace — Meet, Collaborate, Communicate, Work",
    description: "Secure meetings, chat, files, rooms, and teamwork in one adaptive workspace.",
    images: [{ url: "/opengraph-image", width: 1200, height: 630, alt: "Xspace secure collaboration platform" }],
  },
  twitter: {
    card: "summary_large_image",
    title: "Xspace — Meet, Collaborate, Communicate, Work",
    description: "Secure meetings, chat, files, rooms, and teamwork in one adaptive workspace.",
    images: ["/opengraph-image"],
  },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="id" className="h-full antialiased" data-theme="dark-green" suppressHydrationWarning>
      <body className="min-h-full flex flex-col" suppressHydrationWarning>
        <script dangerouslySetInnerHTML={{__html:`try{var theme=localStorage.getItem("xpace-theme");if(theme!=="light"&&theme!=="dark-green")theme="dark-green";document.documentElement.dataset.theme=theme;document.documentElement.style.colorScheme=theme==="light"?"light":"dark"}catch(e){document.documentElement.dataset.theme="dark-green"}`}}/>
        {children}
      </body>
    </html>
  );
}
