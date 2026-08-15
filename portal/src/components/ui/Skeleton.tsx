/* Hallmark · component: skeleton · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * แทน spinner กลางหน้า — เนื้อหาที่รู้รูปร่างล่วงหน้าต้องไม่ทำ layout กระโดด
 */
import React from 'react';

export function SkeletonLine({ width = '100%', className = '' }: { width?: string; className?: string }) {
  return <span className={`ui-skeleton block h-3 ${className}`} style={{ width }} aria-hidden="true" />;
}

/** ใช้แทนบล็อกสถิติหรือการ์ดสรุป */
export function SkeletonCard({ lines = 3 }: { lines?: number }) {
  return (
    <div className="ui-card p-4" aria-hidden="true">
      <SkeletonLine width="40%" className="mb-3 h-4" />
      <div className="space-y-2">
        {Array.from({ length: lines }).map((_, i) => (
          <SkeletonLine key={i} width={i === lines - 1 ? '60%' : '100%'} />
        ))}
      </div>
    </div>
  );
}

/** ใช้แทนตารางที่กำลังโหลด — จำนวนแถวควรเท่าจำนวนที่หน้าแสดงจริง */
export function SkeletonRows({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="space-y-2 p-4" aria-hidden="true">
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-3">
          {Array.from({ length: cols }).map((_, c) => (
            <SkeletonLine key={c} width={c === 0 ? '28%' : '18%'} />
          ))}
        </div>
      ))}
    </div>
  );
}

/** ครอบส่วนที่กำลังโหลด บอก screen reader ว่ากำลังโหลดอยู่ */
export function Loading({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div role="status" aria-live="polite" aria-label={label}>
      {children}
    </div>
  );
}
