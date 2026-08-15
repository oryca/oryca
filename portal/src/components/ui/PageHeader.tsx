/* Hallmark · component: page-header · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * หัวหน้าเดียวทั้งแอป — หนึ่ง h1 ต่อหน้า ไม่มี eyebrow ไม่มีเลข section
 */
import React from 'react';

export interface PageHeaderProps {
  title: string;
  description?: string;
  /** ปุ่มมุมขวา — ปุ่ม primary ได้แค่หนึ่ง */
  actions?: React.ReactNode;
}

export function PageHeader({ title, description, actions }: PageHeaderProps) {
  return (
    <header className="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div className="min-w-0">
        <h1 className="font-title text-xl font-semibold text-ink">{title}</h1>
        {description && (
          <p className="mt-1 max-w-prose text-sm text-muted">{description}</p>
        )}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}
