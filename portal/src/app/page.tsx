'use client';

import React, { useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { useAuth } from '@/app/providers';
import Link from 'next/link';
import { Shield, ArrowRight, Server, Key, BarChart3, Mail } from 'lucide-react';

export default function LandingPage() {
  const router = useRouter();
  const { isAuthenticated, isLoading, config } = useAuth();

  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, isLoading, router]);

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper-2">
        <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
      </div>
    );
  }

  const registrationOpen = config?.register?.enabled !== false;

  return (
    <div className="flex flex-col min-h-screen bg-paper">
      {/* Top Header */}
      <header className="border-b border-rule bg-paper sticky top-0 z-40">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="font-title text-xl font-bold tracking-tight text-ink">
              oryca<span className="text-accent">.</span>portal
            </span>
          </div>

          <div className="flex items-center gap-4">
            <Link
              href="/auth/login"
              className="text-xs font-semibold text-ink hover:text-accent transition duration-short"
            >
              Sign In
            </Link>
            {registrationOpen && (
              <Link
                href="/auth/register"
                className="px-3.5 py-1.5 bg-accent hover:bg-accent-deep text-accent-ink text-xs font-semibold rounded-control transition duration-short"
              >
                Sign Up
              </Link>
            )}
          </div>
        </div>
      </header>

      {/* Hero Section */}
      <main className="flex-grow flex flex-col justify-center max-w-5xl mx-auto px-4 py-16 sm:px-6 lg:px-8 gap-12">
        <div className="text-center space-y-4">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-control bg-accent-wash border border-accent-edge text-accent mb-2">
            <Shield className="w-6 h-6" />
          </div>
          <h1 className="font-title text-4xl sm:text-5xl font-extrabold tracking-tight text-ink max-w-3xl mx-auto leading-tight">
            Geospatial API gateway <span className="text-accent">developer portal</span>
          </h1>
          <p className="text-sm sm:text-base text-muted max-w-xl mx-auto leading-relaxed">
            Put ORYCA in front of your spatial services to manage authentication, rate limiting, and response link rewrites.
          </p>

          <div className="pt-4 flex justify-center gap-4">
            <Link
              href="/auth/login"
              className="px-5 py-2.5 bg-accent hover:bg-accent-deep text-accent-ink text-sm font-semibold rounded-control transition duration-short flex items-center gap-2 shadow-sm"
            >
              Get API Keys <ArrowRight className="w-4 h-4" />
            </Link>
          </div>
        </div>

        {/* Product Features Pitch */}
        <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
          <div className="bg-paper border border-rule rounded-surface p-5 shadow-sm space-y-2">
            <div className="w-8 h-8 rounded-control bg-accent-wash border border-accent-edge flex items-center justify-center text-accent">
              <Server className="w-4 h-4" />
            </div>
            <h3 className="font-title text-sm font-bold text-ink">Service Catalog</h3>
            <p className="text-xs text-muted leading-relaxed">
              Browse OGC Features, STAC, Styles, SensorThings, and Tiles specifications directly.
            </p>
          </div>

          <div className="bg-paper border border-rule rounded-surface p-5 shadow-sm space-y-2">
            <div className="w-8 h-8 rounded-control bg-accent-wash border border-accent-edge flex items-center justify-center text-accent">
              <Key className="w-4 h-4" />
            </div>
            <h3 className="font-title text-sm font-bold text-ink">API Key Access</h3>
            <p className="text-xs text-muted leading-relaxed">
              Instantly generate, regenerate, or revoke developer keys and test endpoints directly.
            </p>
          </div>

          <div className="bg-paper border border-rule rounded-surface p-5 shadow-sm space-y-2">
            <div className="w-8 h-8 rounded-control bg-accent-wash border border-accent-edge flex items-center justify-center text-accent">
              <BarChart3 className="w-4 h-4" />
            </div>
            <h3 className="font-title text-sm font-bold text-ink">Logs Diagnostic</h3>
            <p className="text-xs text-muted leading-relaxed">
              Track requests speed, statuses, and diagnose rate-limiting tiers through interactive charts.
            </p>
          </div>

          <div className="bg-paper border border-rule rounded-surface p-5 shadow-sm space-y-2">
            <div className="w-8 h-8 rounded-control bg-accent-wash border border-accent-edge flex items-center justify-center text-accent">
              <Mail className="w-4 h-4" />
            </div>
            <h3 className="font-title text-sm font-bold text-ink">Response rewrite</h3>
            <p className="text-xs text-muted leading-relaxed">
              Configure OGC transforms and rewrite upstream hyperlinks dynamically.
            </p>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-rule bg-paper-2 py-6">
        <div className="max-w-7xl mx-auto px-4 text-center text-xs text-muted">
          ORYCA open-source API Gateway (Apache-2.0). All rights reserved.
        </div>
      </footer>
    </div>
  );
}
