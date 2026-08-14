'use client';

import React, { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useAuth } from '@/app/providers';
import { api } from '@/lib/api';
import { Shield, AlertTriangle, CheckCircle, Info } from 'lucide-react';

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { login, isAuthenticated } = useAuth();
  
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isNotVerified, setIsNotVerified] = useState(false);

  useEffect(() => {
    if (isAuthenticated) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, router]);

  useEffect(() => {
    if (searchParams.get('expired')) {
      setInfo('Your session has expired. Please sign in again.');
    } else if (searchParams.get('registered')) {
      setInfo('Registration successful! Since no mail server is configured by default, an administrator must verify your account before you can sign in.');
    } else if (searchParams.get('reset_success')) {
      setInfo('Password has been set successfully. You can now log in.');
    }
  }, [searchParams]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setIsNotVerified(false);
    setIsLoading(true);

    try {
      const res = await api.post('/auth/login', { username, password });
      login(res.data);
      router.push('/dashboard');
    } catch (err: any) {
      console.error(err);
      if (err.response?.status === 403 && err.response?.data?.code === 'USER_NOT_VERIFIED') {
        setIsNotVerified(true);
        setError('Your account is not verified yet. Since there is no mail server configured out of the box, an administrator must manually verify your account before you can sign in.');
      } else {
        setError(err.message || 'Invalid username or password.');
      }
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-2 px-4 py-12 sm:px-6 lg:px-8">
      <div className="w-full max-w-md space-y-8 bg-paper p-8 rounded-surface border border-rule shadow-sm">
        <div className="text-center">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-control bg-accent-wash border border-accent-edge text-accent mb-4">
            <Shield className="w-6 h-6" />
          </div>
          <h2 className="font-title text-2xl font-bold tracking-tight text-ink">
            Sign in to ORYCA
          </h2>
          <p className="mt-2 text-sm text-muted">
            API gateway developer portal & control panel
          </p>
        </div>

        {info && (
          <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok">
            <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <div>{info}</div>
          </div>
        )}

        {error && (
          <div className={`flex items-start gap-3 rounded-control p-4 text-xs ${isNotVerified ? 'border border-warn-edge bg-warn-wash text-warn' : 'border border-danger-edge bg-danger-wash text-danger'}`}>
            {isNotVerified ? (
              <Info className="w-4 h-4 flex-shrink-0 mt-0.5" />
            ) : (
              <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            )}
            <div>{error}</div>
          </div>
        )}

        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          <div className="space-y-4 rounded-md">
            <div>
              <label htmlFor="email-address" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                Email Address or Username
              </label>
              <input
                id="email-address"
                name="email"
                type="text"
                required
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                placeholder="email@example.com"
              />
            </div>
            <div>
              <div className="flex justify-between items-center mb-1">
                <label htmlFor="password" className="text-xs font-semibold text-muted uppercase tracking-wider block">
                  Password
                </label>
                <Link href="/auth/forgot-password" className="text-xs text-accent hover:underline">
                  Forgot Password?
                </Link>
              </div>
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
          </div>

          <div>
            <button
              type="submit"
              disabled={isLoading}
              className="relative flex w-full justify-center rounded-control bg-accent hover:bg-accent-deep text-accent-ink py-2.5 text-sm font-semibold transition disabled:opacity-50"
            >
              {isLoading ? 'Signing in...' : 'Sign In'}
            </button>
          </div>
        </form>

        <div className="text-center text-xs text-muted border-t border-rule pt-4">
          Need an account?{' '}
          <Link href="/auth/register" className="font-semibold text-accent hover:underline">
            Create account
          </Link>
        </div>
      </div>
    </div>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-screen items-center justify-center bg-paper-2">
        <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
      </div>
    }>
      <LoginForm />
    </Suspense>
  );
}
