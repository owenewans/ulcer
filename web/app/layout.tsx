import type { Metadata, Viewport } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Ulcer control plane",
  description: "Operate Ulcer instances, runtime adapters, and traffic from the control panel.",
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
