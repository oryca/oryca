'use client';

import React, { useState } from 'react';
import Link from 'next/link';
import { api } from '@/lib/api';
import { Shield, AlertTriangle, CheckCircle, Mail } from 'lucide-react';

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);
    setIsLoading(true);

    try {
      await api.post('/auth/forgot-password', { email });
      setSuccess('If this email is registered, we have sent instructions to reset your password. (Since SMTP settings might not be configured, you can check the control-plane container logs for the reset link!)');
    } catch (err: any) {
      console.error(err);
      setError(err.message || 'Failed to send password reset request.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-paper-2 px-4 py-12 sm:px-6 lg:px-8">
      <div className="w-full max-w-md space-y-8 bg-paper p-8 rounded-surface border border-rule shadow-sm">
        <div className="text-center">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-control bg-accent-wash border border-accent-edge text-accent mb-4">
            <Mail className="w-6 h-6" />
          </div>
          <h2 className="font-title text-2xl font-bold tracking-tight text-ink">
            Reset Password
          </h2>
          <p className="mt-2 text-sm text-muted">
            Enter your email to request a reset token
          </p>
        </div>

        {success && (
          <div className="flex items-start gap-3 rounded-control border border-ok-edge bg-ok-wash p-4 text-xs text-ok">
            <CheckCircle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <div>{success}</div>
          </div>
        )}

        {error && (
          <div className="flex items-start gap-3 rounded-control border border-danger-edge bg-danger-wash p-4 text-xs text-danger">
            <AlertTriangle className="w-4 h-4 flex-shrink-0 mt-0.5" />
            <div>{error}</div>
          </div>
        )}

        {!success && (
          <form className="mt-8 space-y-6" onSubmit={handleSubmit}>
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
              <button
                type="submit"
                disabled={isLoading}
                className="relative flex w-full justify-center rounded-control bg-accent hover:bg-accent-deep text-accent-ink py-2.5 text-sm font-semibold transition disabled:opacity-50"
              >
                {isLoading ? 'Sending...' : 'Send Reset Link'}
              </button>
            </div>
          </form>
        )}

        <div className="text-center text-xs text-muted border-t border-rule pt-4">
          <Link href="/auth/login" className="font-semibold text-accent hover:underline">
            Back to Sign In
          </Link>
        </div>
      </div>
    </div>
  );
}
