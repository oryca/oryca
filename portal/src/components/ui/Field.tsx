/* Hallmark · component: field · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * states: default · hover · focus · active · disabled · loading · error · success
 * label อยู่บนเสมอ · helper จองความสูงไว้ 1 บรรทัด · error มาแทน helper ไม่ดันหน้า
 */
'use client';

import React, { useId } from 'react';

interface BaseProps {
  label: string;
  /** คำอธิบายใต้ช่อง — ถ้ามี error จะถูกแทนที่ */
  helper?: string;
  error?: string;
  success?: string;
  required?: boolean;
  /** class ของตัวช่องกรอกเอง — className ไปลงที่กล่องนอก ไม่ถึง input */
  inputClassName?: string;
}

export interface FieldProps
  extends BaseProps,
    Omit<React.InputHTMLAttributes<HTMLInputElement>, 'required'> {}

export interface TextAreaFieldProps
  extends BaseProps,
    Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'required'> {}

function note(error?: string, success?: string, helper?: string) {
  if (error) return { text: error, cls: 'ui-field__note--error', live: true };
  if (success) return { text: success, cls: 'ui-field__note--ok', live: true };
  return { text: helper ?? '', cls: '', live: false };
}

export function Field({
  label,
  helper,
  error,
  success,
  required,
  className = '',
  inputClassName = '',
  id,
  ...rest
}: FieldProps) {
  const auto = useId();
  const inputId = id ?? auto;
  const noteId = `${inputId}-note`;
  const n = note(error, success, helper);

  return (
    <div className={className}>
      <label className="ui-field__label" htmlFor={inputId}>
        {label}
        {required && <span className="ui-field__req" aria-hidden="true">*</span>}
      </label>
      <input
        {...rest}
        id={inputId}
        className={`ui-input ${inputClassName}`}
        required={required}
        aria-required={required || undefined}
        aria-invalid={error ? true : undefined}
        aria-describedby={n.text ? noteId : undefined}
        data-state={success && !error ? 'success' : undefined}
      />
      <span id={noteId} className={`ui-field__note ${n.cls}`} aria-live={n.live ? 'polite' : undefined}>
        {n.text}
      </span>
    </div>
  );
}

export function TextAreaField({
  label,
  helper,
  error,
  success,
  required,
  className = '',
  inputClassName = '',
  id,
  ...rest
}: TextAreaFieldProps) {
  const auto = useId();
  const inputId = id ?? auto;
  const noteId = `${inputId}-note`;
  const n = note(error, success, helper);

  return (
    <div className={className}>
      <label className="ui-field__label" htmlFor={inputId}>
        {label}
        {required && <span className="ui-field__req" aria-hidden="true">*</span>}
      </label>
      <textarea
        {...rest}
        id={inputId}
        className={`ui-input ${inputClassName}`}
        required={required}
        aria-required={required || undefined}
        aria-invalid={error ? true : undefined}
        aria-describedby={n.text ? noteId : undefined}
      />
      <span id={noteId} className={`ui-field__note ${n.cls}`} aria-live={n.live ? 'polite' : undefined}>
        {n.text}
      </span>
    </div>
  );
}
