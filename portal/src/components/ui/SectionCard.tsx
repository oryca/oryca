/* Hallmark · component: section-card · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * หัวข้อระดับสองเสียงเดียวทั้งแอป — hairline คั่น ไม่ใช้เงา
 */
import React from 'react';

export interface SectionCardProps {
  title?: string;
  description?: string;
  actions?: React.ReactNode;
  /** เนื้อหาเต็มขอบ เช่น ตาราง — ตัด padding ของ body ออก */
  flush?: boolean;
  className?: string;
  children: React.ReactNode;
}

export function SectionCard({
  title,
  description,
  actions,
  flush = false,
  className = '',
  children,
}: SectionCardProps) {
  return (
    <section className={`ui-card ${className}`}>
      {title && (
        <div className="ui-card__head">
          <div className="min-w-0">
            <h2 className="ui-card__title">{title}</h2>
            {description && <p className="ui-card__desc">{description}</p>}
          </div>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
      )}
      <div className={flush ? '' : 'ui-card__body'}>{children}</div>
    </section>
  );
}
