/* Hallmark · component: notice · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * แถบข้อความในหน้า — ใช้เมื่อข้อความคือสถานะของหน้านั้นเอง ไม่ใช่ผลของ action ที่เพิ่งกด
 * ถ้าเป็นผลของ action ให้ใช้ useToast แทน
 */
import React from 'react';
import { CheckCircle, AlertTriangle, Info, XCircle } from 'lucide-react';

type Tone = 'info' | 'ok' | 'warn' | 'danger';

const TONE = {
  info: { cls: 'border-accent-edge bg-accent-wash text-ink-2', icon: Info, iconCls: 'text-accent' },
  ok: { cls: 'border-ok-edge bg-ok-wash text-ink-2', icon: CheckCircle, iconCls: 'text-ok' },
  warn: { cls: 'border-warn-edge bg-warn-wash text-ink-2', icon: AlertTriangle, iconCls: 'text-warn' },
  danger: { cls: 'border-danger-edge bg-danger-wash text-ink-2', icon: XCircle, iconCls: 'text-danger' },
} as const;

export interface NoticeProps {
  tone?: Tone;
  title?: string;
  children: React.ReactNode;
  className?: string;
}

export function Notice({ tone = 'info', title, children, className = '' }: NoticeProps) {
  const { cls, icon: Icon, iconCls } = TONE[tone];
  return (
    <div
      className={`flex items-start gap-3 rounded-control border p-3 text-sm ${cls} ${className}`}
      role={tone === 'danger' ? 'alert' : 'status'}
    >
      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${iconCls}`} aria-hidden="true" />
      <div className="min-w-0">
        {title && <p className="font-medium text-ink">{title}</p>}
        <div className={title ? 'mt-0.5' : undefined}>{children}</div>
      </div>
    </div>
  );
}
