/* Hallmark · component: select · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * states: default · hover · focus · disabled · error · success
 * The one dropdown in the portal — pages must not hand-roll <select> + chevron.
 */
'use client';

import React, { useId } from 'react';
import { ChevronDown } from 'lucide-react';

export interface SelectProps
  extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, 'size'> {
  size?: 'sm' | 'md';
  /** Renders a label bound to the select by id */
  label?: string;
  /** Lands on the outer box; use controlClassName to reach the select */
  className?: string;
  controlClassName?: string;
  /** React 19 passes ref as a normal prop — no forwardRef */
  ref?: React.Ref<HTMLSelectElement>;
}

export function Select({
  size = 'md',
  label,
  required,
  className = '',
  controlClassName = '',
  id,
  children,
  ...rest
}: SelectProps) {
  const auto = useId();
  const selectId = id ?? auto;

  const control = (
    <select
      {...rest}
      id={selectId}
      className={`ui-input ui-select__control ${controlClassName}`}
      required={required}
      aria-required={required || undefined}
    >
      {children}
    </select>
  );
  const chevron = <ChevronDown className="ui-select__chevron" aria-hidden="true" />;

  const shell = ['ui-select', size === 'sm' && 'ui-select--sm'].filter(Boolean).join(' ');

  if (!label) {
    return (
      <div className={`${shell} ${className}`}>
        {control}
        {chevron}
      </div>
    );
  }

  return (
    <div className={className}>
      <label className="ui-field__label" htmlFor={selectId}>
        {label}
        {required && <span className="ui-field__req" aria-hidden="true">*</span>}
      </label>
      <div className={shell}>
        {control}
        {chevron}
      </div>
    </div>
  );
}
