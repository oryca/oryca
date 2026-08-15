/* Hallmark · component: empty-state · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * ว่างเปล่าต้องบอกสามอย่าง: ไอคอน · ทำไมถึงว่าง · ปุ่มที่ทำให้ไม่ว่าง
 */
import React from 'react';

export interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  /** เหตุผลที่ว่าง หนึ่งประโยค ไม่ใช่ "No data found" */
  description?: string;
  action?: React.ReactNode;
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center px-4 py-10 text-center">
      {icon && (
        <span className="mb-3 flex h-10 w-10 items-center justify-center rounded-control border border-rule bg-paper-2 text-muted">
          {icon}
        </span>
      )}
      <p className="font-title text-base font-semibold text-ink">{title}</p>
      {description && <p className="mt-1 max-w-sm text-sm text-muted">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
