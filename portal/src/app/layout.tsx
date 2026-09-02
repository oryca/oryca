import type { Metadata } from 'next';
import './globals.css';
import { Providers } from './providers';

export const metadata: Metadata = {
  title: 'ORYCA Developer Portal',
  description: 'Developer portal and administration panel for ORYCA API Gateway.',
};

export const dynamic = 'force-dynamic';

function runtimeConfig() {
  return {
    apiUrl:
      process.env.ORYCA_PORTAL_API_URL ||
      process.env.NEXT_PUBLIC_API_URL ||
      'http://localhost:9001/control-plane/api/v1',
    gatewayUrl:
      process.env.ORYCA_PORTAL_GATEWAY_URL ||
      process.env.NEXT_PUBLIC_GATEWAY_URL ||
      'http://localhost:9002/gateway/api',
  };
}

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  // escape < กัน string ที่มี </script> ปิด tag ก่อนเวลา
  const config = JSON.stringify(runtimeConfig()).replace(/</g, '\\u003c');

  return (
    <html lang="en">
      <head>
        <script dangerouslySetInnerHTML={{ __html: `window.__ORYCA_CONFIG__=${config}` }} />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link
          href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;500;600;700&family=Sarabun:wght@300;400;500;600;700&family=JetBrains+Mono:wght@400;500;600&display=swap"
          rel="stylesheet"
        />
      </head>
      <body className="flex min-h-screen flex-col bg-paper text-ink antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
