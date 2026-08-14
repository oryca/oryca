'use client';

import React, { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import { Shield, AlertTriangle, UserPlus } from 'lucide-react';

export default function RegisterPage() {
  const router = useRouter();
  const { config, isLoadingConfig, isAuthenticated } = useAuth();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [firstName, setFirstName] = useState('');
  const [lastName, setLastName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  useEffect(() => {
    if (isAuthenticated) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, router]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsLoading(true);

    try {
      await api.post('/auth/register', {
        email,
        password,
        firstName,
        lastName,
      });
      router.push('/auth/login?registered=true');
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Failed to register account. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  if (isLoadingConfig) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-paper-2">
        <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
      </div>
    );
  }

  const registrationOpen = config?.register?.enabled !== false;

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-2 px-4 py-12 sm:px-6 lg:px-8">
      <div className="w-full max-w-md space-y-8 bg-paper p-8 rounded-surface border border-rule shadow-sm">
        <div className="text-center">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-control bg-accent-wash border border-accent-edge text-accent mb-4">
            <UserPlus className="w-6 h-6" />
          </div>
          <h2 className="font-title text-2xl font-bold tracking-tight text-ink">
            Create account
          </h2>
          <p className="mt-2 text-sm text-muted">
            Register to request access to geospatial APIs
          </p>
        </div>

        {!registrationOpen ? (
          <div className="rounded-control border border-warn-edge bg-warn-wash p-4 text-center">
            <AlertTriangle className="w-5 h-5 text-warn mx-auto mb-2" />
            <p className="text-xs text-warn font-semibold">Registration Closed</p>
            <p className="text-xs text-ink-3 mt-1">
              Account registration is currently closed. If you require access, please contact the administrator.
            </p>
            <div className="mt-6 border-t border-rule pt-4">
              <Link href="/auth/login" className="text-xs font-semibold text-accent hover:underline">
                Back to Sign In
              </Link>
            </div>
          </div>
        ) : (
          <>
            {error && (
              <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
                <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
                <div>{error}</div>
              </div>
            )}

            <form className="mt-8 space-y-4" onSubmit={handleSubmit}>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label htmlFor="first-name" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    First Name
                  </label>
                  <input
                    id="first-name"
                    name="firstName"
                    type="text"
                    required
                    value={firstName}
                    onChange={(e) => setFirstName(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                    placeholder="John"
                  />
                </div>
                <div>
                  <label htmlFor="last-name" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                    Last Name
                  </label>
                  <input
                    id="last-name"
                    name="lastName"
                    type="text"
                    required
                    value={lastName}
                    onChange={(e) => setLastName(e.target.value)}
                    className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                    placeholder="Doe"
                  />
                </div>
              </div>

              <div>
                <label htmlFor="email-address" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Email Address
                </label>
                <input
                  id="email-address"
                  name="email"
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  placeholder="john.doe@example.com"
                />
              </div>

              <div>
                <label htmlFor="password" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                  Password
                </label>
                <input
                  id="password"
                  name="password"
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                  placeholder="••••••••"
                />
              </div>

              <div className="pt-2">
                <button
                  type="submit"
                  disabled={isLoading}
                  className="relative flex w-full justify-center rounded-control bg-accent hover:bg-accent-deep text-accent-ink py-2.5 text-sm font-semibold transition disabled:opacity-50"
                >
                  {isLoading ? 'Creating account...' : 'Create Account'}
                </button>
              </div>
            </form>

            <div className="text-center text-xs text-muted border-t border-rule pt-4">
              Already have an account?{' '}
              <Link href="/auth/login" className="font-semibold text-accent hover:underline">
                Sign in
              </Link>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
