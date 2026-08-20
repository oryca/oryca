/* Hallmark · macrostructure: Letter · genre: modern-minimal
 * design-system: design.md · designed-as-app
 */
'use client';

import React, { useState, Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import { api } from '@/lib/api';
import { AuthShell, AuthLink, Button, Notice } from '@/components/ui';
import { MailCheck } from 'lucide-react';

function VerifyEmailForm() {
  const searchParams = useSearchParams();
  const token = searchParams.get('token');

  const [isVerifying, setIsVerifying] = useState(false);
  const [verified, setVerified] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function verify() {
    if (!token || isVerifying) return;
    setIsVerifying(true);
    setError(null);
    try {
      await api.get(`/auth/verify-email?token=${token}`);
      setVerified(true);
    } catch (err) {
      const code = (err as { response?: { data?: { code?: string } } })?.response?.data?.code;
      if (code === 'TOKEN_INVALID_OR_EXPIRED') {
        setError(
          'This link has expired, or your email is already verified. ' +
            'If you can sign in, you are verified.',
        );
      } else {
        setError(
          err instanceof Error && err.message
            ? err.message
            : 'Something went wrong while verifying. Please try again.',
        );
      }
    } finally {
      setIsVerifying(false);
    }
  }

  if (!token) {
    return (
      <Notice tone="danger" title="This link will not work">
        The link has no token, or the token was lost on the way.
        <br />
        Sign in and request a fresh verification email, or ask an administrator to verify
        your account.
      </Notice>
    );
  }

  if (verified) {
    return (
      <Notice tone="ok" title="Email verified">
        Your email address is confirmed. You can sign in now.
      </Notice>
    );
  }

  return (
    <>
      {error && (
        <Notice tone="danger" title="Verification failed">
          {error}
        </Notice>
      )}
      <Button variant="primary" block onClick={verify} loading={isVerifying}>
        Verify my email
      </Button>
    </>
  );
}

export default function VerifyEmailPage() {
  return (
    <AuthShell
      icon={<MailCheck className="h-6 w-6" />}
      title="Verify your email"
      description="Confirming your email address"
      footer={<AuthLink href="/auth/login">Back to sign in</AuthLink>}
    >
      <Suspense fallback={<Button variant="primary" block disabled>Loading</Button>}>
        <VerifyEmailForm />
      </Suspense>
    </AuthShell>
  );
}
