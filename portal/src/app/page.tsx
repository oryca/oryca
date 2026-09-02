/* Hallmark · macrostructure: Marquee Hero · genre: modern-minimal
 * design-system: design.md · designed-as-app
 * ORYCA developer portal entry
 */
'use client';

import React, { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useAuth } from '@/app/providers';
import { SkeletonLine } from '@/components/ui';
import { Shield, ArrowRight, Server, Key, BarChart3, Shuffle } from 'lucide-react';

const CAPABILITIES = [
  {
    icon: Server,
    title: 'Service catalog',
    body: 'Browse services and routes published through the gateway.',
  },
  {
    icon: Key,
    title: 'API keys',
    body: 'Create and manage keys to call endpoints from your applications.',
  },
  {
    icon: BarChart3,
    title: 'Request logs',
    body: 'View request volume, response times, and status codes for your calls.',
  },
  {
    icon: Shuffle,
    title: 'Response transforms',
    body: 'Rewrite links and headers to keep traffic routing through the gateway.',
  },
];

export default function LandingPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading, config } = useAuth();

  useEffect(() => {
    if (!isLoading && isAuthenticated) router.push('/dashboard');
  }, [isAuthenticated, isLoading, router]);

  if (isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-paper-2">
        <output className="w-full max-w-sm space-y-3 px-6">
          <SkeletonLine className="h-8" />
          <SkeletonLine width="70%" />
        </output>
      </main>
    );
  }

  const registrationOpen = config?.register?.enabled !== false;

  return (
    <div className="relative flex min-h-screen flex-col overflow-hidden bg-paper text-ink">
      {/* ================= Subtle Origami Background ================= */}
      <svg
        aria-hidden="true"
        className="pointer-events-none absolute -top-12 -left-16 h-[720px] w-[560px] opacity-60 sm:h-[840px] sm:w-[680px]"
        viewBox="0 0 600 800"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <polygon points="0,0 240,80 120,280" fill="oklch(91% 0.045 254 / 0.35)" stroke="oklch(84% 0.06 254 / 0.4)" strokeWidth="0.75" />
        <polygon points="240,80 420,0 360,200" fill="oklch(93% 0.035 254 / 0.3)" stroke="oklch(85% 0.05 254 / 0.3)" strokeWidth="0.75" />
        <polygon points="240,80 360,200 120,280" fill="oklch(88% 0.06 254 / 0.4)" stroke="oklch(80% 0.08 254 / 0.5)" strokeWidth="0.75" />
        <polygon points="0,0 120,280 0,380" fill="oklch(86% 0.07 254 / 0.3)" stroke="oklch(79% 0.09 254 / 0.4)" strokeWidth="0.75" />
        <polygon points="120,280 360,200 280,440" fill="oklch(92% 0.04 254 / 0.25)" stroke="oklch(82% 0.07 254 / 0.3)" strokeWidth="0.75" />
        <polygon points="0,380 120,280 180,520" fill="oklch(90% 0.05 254 / 0.35)" stroke="oklch(81% 0.08 254 / 0.4)" strokeWidth="0.75" />
        <polygon points="120,280 280,440 180,520" fill="oklch(87% 0.07 254 / 0.3)" stroke="oklch(78% 0.09 254 / 0.4)" strokeWidth="0.75" />
        <polygon points="360,200 480,260 280,440" fill="oklch(95% 0.025 254 / 0.2)" stroke="oklch(86% 0.04 254 / 0.25)" strokeWidth="0.75" />
        <polygon points="180,520 280,440 220,680" fill="oklch(93% 0.035 254 / 0.25)" stroke="oklch(84% 0.06 254 / 0.3)" strokeWidth="0.75" />
      </svg>

      <svg
        aria-hidden="true"
        className="pointer-events-none absolute -right-20 -bottom-20 h-[640px] w-[640px] opacity-50"
        viewBox="0 0 600 600"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <polygon points="600,600 420,520 540,360" fill="oklch(93% 0.04 78 / 0.35)" stroke="oklch(84% 0.07 75 / 0.3)" strokeWidth="0.75" />
        <polygon points="420,520 280,600 360,420" fill="oklch(95% 0.03 78 / 0.3)" stroke="oklch(86% 0.05 75 / 0.25)" strokeWidth="0.75" />
        <polygon points="420,520 360,420 540,360" fill="oklch(91% 0.055 78 / 0.3)" stroke="oklch(82% 0.08 75 / 0.35)" strokeWidth="0.75" />
        <polygon points="540,360 360,420 480,240" fill="oklch(96% 0.02 78 / 0.2)" stroke="oklch(88% 0.04 75 / 0.2)" strokeWidth="0.75" />
      </svg>

      <header className="relative z-20 border-b border-rule/70 bg-paper/80 backdrop-blur-xs">
        <div className="mx-auto flex h-16 max-w-5xl items-center justify-between px-4 sm:px-6">
          <span className="font-title text-lg font-semibold tracking-tight text-ink">
            oryca<span className="text-accent">.</span>portal
          </span>

          <nav aria-label="Account" className="flex items-center gap-2">
            <Link href="/auth/login" className="ui-btn ui-btn--sm ui-btn--ghost">
              Sign in
            </Link>
            {registrationOpen && (
              <Link href="/auth/register" className="ui-btn ui-btn--sm ui-btn--primary">
                Sign up
              </Link>
            )}
          </nav>
        </div>
      </header>

      <main className="relative z-10 mx-auto flex w-full max-w-5xl flex-grow flex-col justify-center gap-12 px-4 py-16 sm:px-6">
        <section className="max-w-2xl">
          <span
            aria-hidden="true"
            className="mb-5 inline-flex h-12 w-12 items-center justify-center rounded-control border border-accent-edge bg-accent-wash text-accent shadow-xs"
          >
            <Shield className="h-6 w-6" />
          </span>

          <h1 className="font-title text-4xl leading-tight font-semibold text-ink sm:text-5xl">
            ORYCA <span className="text-accent">Portal</span>
          </h1>

          <p className="mt-4 max-w-xl text-base leading-relaxed text-ink-3">
            Browse published services, manage your API keys, and test endpoints through the gateway.
          </p>

          <div className="mt-8 flex flex-wrap items-center gap-3">
            <Link href="/auth/login" className="ui-btn ui-btn--primary shadow-xs">
              Sign in
              <ArrowRight className="h-4 w-4" aria-hidden="true" />
            </Link>
            {registrationOpen && (
              <Link href="/auth/register" className="ui-btn ui-btn--secondary">
                Create account
              </Link>
            )}
          </div>
        </section>

        <section aria-label="What the portal does" className="border-t border-rule/80">
          <dl className="grid grid-cols-1 md:grid-cols-2">
            {CAPABILITIES.map(({ icon: Icon, title, body }) => (
              <div
                key={title}
                className="border-b border-rule/80 py-6 md:odd:border-r md:odd:pr-8 md:even:pl-8"
              >
                <dt className="flex items-center gap-2">
                  <Icon className="h-4 w-4 shrink-0 text-accent" aria-hidden="true" />
                  <span className="font-title text-base font-semibold text-ink">{title}</span>
                </dt>
                <dd className="mt-1.5 text-sm leading-relaxed text-muted">{body}</dd>
              </div>
            ))}
          </dl>
        </section>
      </main>

      <footer className="relative z-10 border-t border-rule bg-paper-2/90 py-6 backdrop-blur-xs">
        <p className="mx-auto max-w-5xl px-4 text-center text-xs text-muted sm:px-6">
          ORYCA — open-source API gateway, licensed under Apache-2.0.
        </p>
      </footer>
    </div>
  );
}

