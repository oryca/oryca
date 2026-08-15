/* Hallmark · macrostructure: Letter · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * design-system: design.md · designed-as-app
 * เปลือกของหน้า auth ทั้งสี่หน้า — คอลัมน์เดียวแคบ หนึ่งจอหนึ่งงาน ไม่มีเมนู
 */
import React from 'react';
import Link from 'next/link';

export interface AuthShellProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  children: React.ReactNode;
  /** ลิงก์ท้ายการ์ด เช่น "Back to sign in" */
  footer?: React.ReactNode;
}

export function AuthShell({ icon, title, description, children, footer }: AuthShellProps) {
  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-paper-2 px-4 py-12">
      {/* Subtle Background Origami Mesh */}
      <svg
        aria-hidden="true"
        className="pointer-events-none absolute -top-16 -left-16 h-96 w-96 opacity-40"
        viewBox="0 0 400 400"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <polygon points="0,0 180,60 90,210" fill="oklch(91% 0.045 254 / 0.4)" stroke="oklch(84% 0.06 254 / 0.4)" strokeWidth="0.75" />
        <polygon points="180,60 320,0 270,150" fill="oklch(93% 0.035 254 / 0.35)" stroke="oklch(85% 0.05 254 / 0.35)" strokeWidth="0.75" />
        <polygon points="180,60 270,150 90,210" fill="oklch(88% 0.06 254 / 0.4)" stroke="oklch(80% 0.08 254 / 0.5)" strokeWidth="0.75" />
        <polygon points="0,0 90,210 0,280" fill="oklch(86% 0.07 254 / 0.35)" stroke="oklch(79% 0.09 254 / 0.4)" strokeWidth="0.75" />
      </svg>
      <svg
        aria-hidden="true"
        className="pointer-events-none absolute -right-16 -bottom-16 h-96 w-96 opacity-35"
        viewBox="0 0 400 400"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <polygon points="400,400 280,350 360,240" fill="oklch(93% 0.04 78 / 0.4)" stroke="oklch(84% 0.07 75 / 0.35)" strokeWidth="0.75" />
        <polygon points="280,350 180,400 240,280" fill="oklch(95% 0.03 78 / 0.35)" stroke="oklch(86% 0.05 75 / 0.3)" strokeWidth="0.75" />
      </svg>

      <div className="relative z-10 w-full max-w-md rounded-surface border border-rule bg-paper/95 p-6 shadow-xs backdrop-blur-xs sm:p-8">
        <div className="text-center">
          <span
            aria-hidden="true"
            className="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-control border border-accent-edge bg-accent-wash text-accent"
          >
            {icon}
          </span>
          <h1 className="font-title text-xl font-semibold text-ink">{title}</h1>
          <p className="mt-1 text-sm text-muted">{description}</p>
        </div>

        <div className="mt-6 space-y-4">{children}</div>

        {footer && (
          <div className="mt-6 border-t border-rule pt-4 text-center text-sm text-muted">
            {footer}
          </div>
        )}
      </div>
    </main>
  );
}

/** ลิงก์ในบรรทัดท้ายการ์ด — ห้ามตัดสองบรรทัด */
export function AuthLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Link
      href={href}
      className="rounded-chip font-medium whitespace-nowrap text-accent underline underline-offset-2"
    >
      {children}
    </Link>
  );
}
