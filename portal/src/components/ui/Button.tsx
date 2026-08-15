/* Hallmark · component: button · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * states: default · hover · focus · active · disabled · loading · error · success
 * design-system: design.md · designed-as-app
 */
import React from 'react';

type Variant = 'primary' | 'secondary' | 'ghost' | 'ghost-danger' | 'danger' | 'danger-solid';

export interface ButtonProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  variant?: Variant;
  size?: 'sm' | 'md';
  /** ระหว่างส่งคำขอ — spinner แทนที่ไอคอน label ยังอ่านได้ ปุ่มกดซ้ำไม่ได้ */
  loading?: boolean;
  /** ผลลัพธ์ล่าสุดของปุ่มนี้ ใช้คู่กับข้อความเสมอ ห้ามสื่อด้วยสีอย่างเดียว */
  state?: 'idle' | 'error' | 'success';
  icon?: React.ReactNode;
  block?: boolean;
  children?: React.ReactNode;
  /** React 19 ส่ง ref เป็น prop ปกติ ไม่ต้อง forwardRef */
  ref?: React.Ref<HTMLButtonElement>;
}

export function Button({
  variant = 'secondary',
  size = 'md',
  loading = false,
  state = 'idle',
  icon,
  block = false,
  disabled,
  className = '',
  type = 'button',
  children,
  ...rest
}: ButtonProps) {
  const iconOnly = !children && (icon || loading);

  let dataState: string | undefined;
  if (loading) dataState = 'loading';
  else if (state !== 'idle') dataState = state;

  const classes = [
    'ui-btn',
    `ui-btn--${variant}`,
    size === 'sm' && 'ui-btn--sm',
    iconOnly && 'ui-btn--icon',
    block && 'ui-btn--block',
    className,
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <button
      {...rest}
      className={classes}
      disabled={disabled || loading}
      aria-disabled={disabled || loading || undefined}
      aria-busy={loading || undefined}
      data-state={dataState}
      type={type}
    >
      {loading ? <span className="ui-btn__spinner" aria-hidden="true" /> : icon}
      {children}
    </button>
  );
}
