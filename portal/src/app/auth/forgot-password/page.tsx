/* Hallmark · macrostructure: Letter · genre: modern-minimal
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useState } from 'react';
import { api } from '@/lib/api';
import { AuthShell, AuthLink, Button, Field, Notice } from '@/components/ui';
import { Mail } from 'lucide-react';

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);
  const [mailNotConfigured, setMailNotConfigured] = useState(false);
  const [isLoading, setIsLoading] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setMailNotConfigured(false);
    setIsLoading(true);

    try {
      await api.post('/auth/forgot-password', {
        email,
        callbackUrl: `${window.location.origin}/auth/set-password`,
      });
      setSent(true);
    } catch (err) {
      const code = (err as { response?: { data?: { code?: string } } })?.response?.data?.code;
      if (code === 'MAIL_NOT_CONFIGURED') {
        setMailNotConfigured(true);
      } else {
        setError(
          err instanceof Error && err.message ? err.message : 'Could not send the reset request.',
        );
      }
    } finally {
      setIsLoading(false);
    }
  }

  return (
    <AuthShell
      icon={<Mail className="h-6 w-6" />}
      title="Reset your password"
      description="Enter your email and we will send a reset link"
      footer={<AuthLink href="/auth/login">Back to sign in</AuthLink>}
    >
      {sent && (
        <Notice tone="ok" title="Request sent">
          If that email is registered, a reset link is on its way.
          <br />
          If email is not configured, ask an administrator to reset your password.
        </Notice>
      )}

      {mailNotConfigured && (
        <Notice tone="warn" title="Email is not configured">
          The administrator has not set up email for this site yet, so the reset link
          cannot be sent. Ask an administrator to reset your password for you.
        </Notice>
      )}

      {error && <Notice tone="danger">{error}</Notice>}

      {!sent && (
        <form className="space-y-4" onSubmit={handleSubmit}>
          <Field
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            autoComplete="email"
            required
          />
          <Button type="submit" variant="primary" block loading={isLoading}>
            Send reset link
          </Button>
        </form>
      )}
    </AuthShell>
  );
}
