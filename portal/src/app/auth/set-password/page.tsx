'use client';

import React, { useState, useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { api } from '@/lib/api';
import { Shield, AlertTriangle, KeyRound, CheckCircle } from 'lucide-react';

function SetPasswordForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get('token');

  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isValidating, setIsValidating] = useState(true);
  const [isTokenValid, setIsTokenValid] = useState(false);
  const [tokenUser, setTokenUser] = useState<{ email: string; firstName: string } | null>(null);

  useEffect(() => {
    if (!token) {
      setError('Invalid or missing password reset token.');
      setIsValidating(false);
      return;
    }

    async function validateToken() {
      try {
        const res = await api.get(`/auth/set-password?token=${token}`);
        setTokenUser(res.data);
        setIsTokenValid(true);
      } catch (err: any) {
        console.error(err);
        setError(err.message || 'The password reset token is invalid or has expired.');
      } finally {
        setIsValidating(false);
      }
    }

    validateToken();
  }, [token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (password !== confirmPassword) {
      setError('Passwords do not match.');
      return;
    }

    setIsLoading(true);

    try {
      await api.post('/auth/set-password', {
        token,
        password,
      });
      router.push('/auth/login?reset_success=true');
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Failed to update password. Please try again.');
    } finally {
      setIsLoading(false);
    }
  };

  if (isValidating) {
    return (
      <div className="flex items-center justify-center p-8">
        <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
      </div>
    );
  }

  return (
    <>
      {error && !isTokenValid && (
        <div className="rounded-control border border-danger-edge bg-danger-wash p-4 text-center">
          <AlertTriangle className="w-5 h-5 text-danger mx-auto mb-2" />
          <p className="text-xs text-danger font-semibold">Validation Failed</p>
          <p className="text-xs text-ink-3 mt-1">{error}</p>
        </div>
      )}

      {isTokenValid && (
        <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
          {tokenUser && (
            <div className="text-xs text-muted mb-4 bg-paper-2 p-3 rounded-control border border-rule">
              Setting new password for <span className="font-semibold text-ink">{tokenUser.firstName} ({tokenUser.email})</span>
            </div>
          )}

          {error && (
            <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
              <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
              <div>{error}</div>
            </div>
          )}

          <div className="space-y-4">
            <div>
              <label htmlFor="password" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                New Password
              </label>
              <input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className="w-full bg-paper-2 border border-rule rounded-control px-3 py-2 text-sm text-ink outline-none transition focus:border-focus focus:ring-1 focus:ring-focus font-sans"
                placeholder="••••••••"
              />
            </div>
            <div>
              <label htmlFor="confirm-password" className="text-xs font-semibold text-muted uppercase tracking-wider block mb-1">
                Confirm New Password
              </label>
              <input
                id="confirm-password"
                type="password"
                required
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
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
              {isLoading ? 'Updating password...' : 'Update Password'}
            </button>
          </div>
        </form>
      )}
    </>
  );
}

export default function SetPasswordPage() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-2 px-4 py-12 sm:px-6 lg:px-8">
      <div className="w-full max-w-md space-y-8 bg-paper p-8 rounded-surface border border-rule shadow-sm">
        <div className="text-center">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-control bg-accent-wash border border-accent-edge text-accent mb-4">
            <KeyRound className="w-6 h-6" />
          </div>
          <h2 className="font-title text-2xl font-bold tracking-tight text-ink">
            Set New Password
          </h2>
          <p className="mt-2 text-sm text-muted">
            Enter your new secure password
          </p>
        </div>

        <Suspense fallback={
          <div className="flex items-center justify-center p-8">
            <div className="w-8 h-8 border-2 border-rule border-t-accent rounded-full animate-spin"></div>
          </div>
        }>
          <SetPasswordForm />
        </Suspense>
      </div>
    </div>
  );
}
