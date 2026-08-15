/* Hallmark · component: toast · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * แทน alert() และ mutation ที่เคยเงียบ
 * กติกา: สำเร็จแล้วเห็นผลอยู่แล้ว = ไม่ต้อง toast · ล้มเหลว = ค้างไว้จนกดปิด
 */
'use client';

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { X } from 'lucide-react';

type Tone = 'ok' | 'error' | 'info';

interface ToastAction {
  label: string;
  run: () => void | Promise<void>;
}

interface ToastInput {
  tone?: Tone;
  message: string;
  action?: ToastAction;
  /** 0 = ค้างไว้จนกดปิด — error ใช้ค่านี้เป็นค่าเริ่มต้น */
  duration?: number;
}

interface ToastItem extends Required<Omit<ToastInput, 'action'>> {
  id: number;
  action?: ToastAction;
}

interface ToastApi {
  toast: (input: ToastInput) => void;
  /** ทำเลย แล้วให้โอกาสกู้คืน 8 วิ — ใช้กับ action ที่ย้อนได้ */
  undoable: (message: string, onUndo: () => void | Promise<void>) => void;
  error: (message: string, retry?: () => void | Promise<void>) => void;
}

const ToastContext = createContext<ToastApi | undefined>(undefined);

export function useToast(): ToastApi {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used inside <ToastProvider>');
  return ctx;
}

let nextId = 1;

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const dismiss = useCallback((id: number) => {
    setItems((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback((input: ToastInput) => {
    const tone = input.tone ?? 'info';
    setItems((prev) => [
      ...prev,
      {
        id: nextId++,
        tone,
        message: input.message,
        action: input.action,
        // ล้มเหลวห้ามหายเอง ผู้ใช้ต้องได้อ่านและได้ลองใหม่
        duration: input.duration ?? (tone === 'error' ? 0 : 4000),
      },
    ]);
  }, []);

  const api = useMemo<ToastApi>(
    () => ({
      toast,
      undoable: (message, onUndo) =>
        toast({ tone: 'info', message, duration: 8000, action: { label: 'Undo', run: onUndo } }),
      error: (message, retry) =>
        toast({
          tone: 'error',
          message,
          action: retry ? { label: 'Try again', run: retry } : undefined,
        }),
    }),
    [toast],
  );

  return (
    <ToastContext.Provider value={api}>
      {children}
      <section className="ui-toast-stack" aria-label="Notifications">
        <div aria-live="polite" aria-atomic="false" className="contents">
          {items.map((t) => (
            <ToastRow key={t.id} item={t} onDismiss={() => dismiss(t.id)} />
          ))}
        </div>
      </section>
    </ToastContext.Provider>
  );
}

function ToastRow({ item, onDismiss }: { item: ToastItem; onDismiss: () => void }) {
  const remaining = useRef(item.duration);
  const startedAt = useRef(0); // ตั้งค่าจริงตอน resume() ไม่ใช่ตอน render
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const clear = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = undefined;
  }, []);

  const resume = useCallback(() => {
    if (remaining.current <= 0) return;
    startedAt.current = Date.now();
    timer.current = setTimeout(onDismiss, remaining.current);
  }, [onDismiss]);

  const pause = useCallback(() => {
    if (remaining.current <= 0) return;
    clear();
    remaining.current -= Date.now() - startedAt.current;
  }, [clear]);

  useEffect(() => {
    resume();
    return clear;
  }, [resume, clear]);

  return (
    <div
      className={`ui-toast ui-rise ui-toast--${item.tone}`}
      onMouseEnter={pause}
      onMouseLeave={resume}
      onFocusCapture={pause}
      onBlurCapture={resume}
    >
      <span className="min-w-0 flex-1 break-words">{item.message}</span>
      {item.action && (
        <button
          type="button"
          className="shrink-0 rounded-chip px-1 text-sm font-semibold whitespace-nowrap text-accent"
          onClick={async () => {
            clear();
            onDismiss();
            await item.action!.run();
          }}
        >
          {item.action.label}
        </button>
      )}
      <button
        type="button"
        className="shrink-0 rounded-chip p-0.5 text-muted"
        aria-label="Dismiss notification"
        onClick={() => {
          clear();
          onDismiss();
        }}
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  );
}
