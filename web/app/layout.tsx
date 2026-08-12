import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "ulcer control plane",
  description: "Protocol infrastructure without ceremony.",
};

export const viewport: Viewport = {
  colorScheme: "dark",
  themeColor: "#090c0d",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
