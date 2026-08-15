/* Hallmark · component: confirm-dialog · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * แทน confirm() ของ browser ทั้งหมด
 * ใช้ <dialog> จริง จึงได้ focus trap · Esc · ::backdrop มาฟรีจากเบราว์เซอร์
 *
 * const confirm = useConfirm();
 * if (await confirm({ title: 'ลบ service', danger: true })) { ... }
 */
'use client';

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from 'react';
import { Button } from './Button';

export interface ConfirmRequest {
  title: string;
  /** อธิบายว่าจะเกิดอะไรขึ้น ไม่ใช่ถามซ้ำว่าแน่ใจไหม */
  description?: string;
  /** ผลกระทบเป็นข้อๆ — ผู้ใช้ควรอ่านจบใน 3 วินาที */
  consequences?: string[];
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  /** ถ้าตั้งไว้ ผู้ใช้ต้องพิมพ์ข้อความนี้ให้ตรงก่อนกดยืนยันได้ */
  typeToConfirm?: string;
}

type Resolver = (ok: boolean) => void;

const ConfirmContext = createContext<((req: ConfirmRequest) => Promise<boolean>) | undefined>(
  undefined,
);

export function useConfirm() {
  const ctx = useContext(ConfirmContext);
  if (!ctx) throw new Error('useConfirm must be used inside <ConfirmProvider>');
  return ctx;
}

export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const [req, setReq] = useState<ConfirmRequest | null>(null);
  const [typed, setTyped] = useState('');
  const resolver = useRef<Resolver | null>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const typedRef = useRef<HTMLInputElement>(null);

  const confirm = useCallback((request: ConfirmRequest) => {
    setTyped('');
    setReq(request);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  // เปิดผ่าน showModal() เท่านั้น — เป็นทางเดียวที่ได้ focus trap และ ::backdrop
  useEffect(() => {
    const el = dialogRef.current;
    if (!el) return;
    if (req && !el.open) {
      el.showModal();
      // โฟกัสตัวแรกที่กดได้ — ช่องพิมพ์ยืนยันถ้ามี ไม่งั้นปุ่มยกเลิก (ตัวเลือกที่ปลอดภัย)
      (typedRef.current ?? cancelRef.current)?.focus();
    }
  }, [req]);

  const settle = useCallback((ok: boolean) => {
    resolver.current?.(ok);
    resolver.current = null;
    dialogRef.current?.close();
    setReq(null);
  }, []);

  // Esc และการปิดด้วยวิธีอื่นของเบราว์เซอร์ = ไม่ยืนยัน
  const onClose = useCallback(() => {
    if (resolver.current) {
      resolver.current(false);
      resolver.current = null;
    }
    setReq(null);
  }, []);

  const needsTyping = Boolean(req?.typeToConfirm);
  const canConfirm = !needsTyping || typed.trim() === req?.typeToConfirm;

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {/* backdrop click = ยกเลิก · ทางลัดคีย์บอร์ดคือ Esc ซึ่ง <dialog> จัดการเองอยู่แล้ว */}
      <dialog
        ref={dialogRef}
        className="ui-dialog"
        onClose={onClose}
        onClick={(e) => {
          // คลิกนอกกล่อง (พื้นที่ ::backdrop) = ยกเลิก
          if (e.target === dialogRef.current) settle(false);
        }}
        aria-labelledby="confirm-title"
      >
        {req && (
          <div className="p-5">
            <h2 id="confirm-title" className="font-title text-base font-semibold text-ink">
              {req.title}
            </h2>

            {req.description && <p className="mt-2 text-sm text-ink-3">{req.description}</p>}

            {req.consequences && req.consequences.length > 0 && (
              <ul className="mt-3 space-y-1.5 rounded-control border border-rule bg-paper-2 p-3 text-sm text-ink-3">
                {req.consequences.map((c) => (
                  <li key={c} className="flex gap-2">
                    <span aria-hidden="true" className="text-muted">
                      —
                    </span>
                    <span className="min-w-0">{c}</span>
                  </li>
                ))}
              </ul>
            )}

            {needsTyping && (
              <label className="mt-4 block">
                <span className="ui-field__label">
                  Type{' '}
                  <code className="font-mono text-ink">{req.typeToConfirm}</code> to confirm
                </span>
                <input
                  ref={typedRef}
                  className="ui-input"
                  value={typed}
                  onChange={(e) => setTyped(e.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
            )}

            <div className="mt-5 flex flex-wrap justify-end gap-2">
              <Button ref={cancelRef} variant="secondary" onClick={() => settle(false)}>
                {req.cancelLabel ?? 'Cancel'}
              </Button>
              <Button
                variant={req.danger ? 'danger-solid' : 'primary'}
                disabled={!canConfirm}
                onClick={() => settle(true)}
              >
                {req.confirmLabel ?? 'Confirm'}
              </Button>
            </div>
          </div>
        )}
      </dialog>
    </ConfirmContext.Provider>
  );
}
