/* Hallmark · component: select · genre: modern-minimal · theme: Cobalt (Thai-adapted)
 * states: default · hover · focus · open · disabled · required
 * The one dropdown in the portal. We draw the option list ourselves so the
 * panel looks the same on every OS; search is opt-in, nothing else differs.
 */
'use client';

import React, { useState, useRef, useEffect, useCallback, useId } from 'react';
import { createPortal } from 'react-dom';
import { Search, ChevronDown, Check, X } from 'lucide-react';

export interface SelectOption {
  value: string;
  label: string;
  subtext?: string;
  badge?: string;
}

/** Options rendered per pass; more load as the list scrolls */
const OPTION_PAGE_SIZE = 20;

/** Room the panel wants below the trigger before it flips above it */
const PANEL_SPACE = 260;

export interface SelectProps {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  /** Blocks submit while empty, same as a native required select */
  required?: boolean;
  size?: 'sm' | 'md';
  /** Adds the search box — worth it once the list outgrows a screenful */
  searchable?: boolean;
  searchPlaceholder?: string;
  /** Lands on the outer box; controlClassName reaches the trigger */
  className?: string;
  controlClassName?: string;
  'aria-label'?: string;
}

type PanelPos = { left: number; width: number; top?: number; bottom?: number };

export function Select({
  label,
  value,
  onChange,
  options = [],
  placeholder = 'Choose...',
  disabled = false,
  required = false,
  size = 'md',
  searchable = false,
  searchPlaceholder = 'Search...',
  className = '',
  controlClassName = '',
  'aria-label': ariaLabel,
}: SelectProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [visibleCount, setVisibleCount] = useState(OPTION_PAGE_SIZE);
  const [activeIndex, setActiveIndex] = useState(-1);
  const [pos, setPos] = useState<PanelPos | null>(null);

  const containerRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const optionRefs = useRef<(HTMLDivElement | null)[]>([]);
  const typeahead = useRef({ text: '', at: 0 });

  const baseId = useId();
  const listId = `${baseId}-list`;
  const labelId = `${baseId}-label`;

  const selectedOption = options.find((opt) => opt.value === value);

  const filteredOptions = options.filter((opt) => {
    if (!searchable || !searchQuery.trim()) return true;
    const query = searchQuery.toLowerCase();
    return (
      opt.label.toLowerCase().includes(query) ||
      opt.subtext?.toLowerCase().includes(query) ||
      opt.badge?.toLowerCase().includes(query)
    );
  });

  function handleOptionsScroll(e: React.UIEvent<HTMLDivElement>) {
    const el = e.currentTarget;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 32) {
      setVisibleCount((count) =>
        count >= filteredOptions.length ? count : count + OPTION_PAGE_SIZE,
      );
    }
  }

  /* Panel is portalled to <body> — several call sites sit inside modals or
   * max-height scroll boxes that would clip an absolutely placed panel. */
  const measure = useCallback(() => {
    const el = triggerRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const below = window.innerHeight - r.bottom;
    const flip = below < PANEL_SPACE && r.top > below;
    setPos({
      left: r.left,
      width: r.width,
      top: flip ? undefined : r.bottom + 4,
      bottom: flip ? window.innerHeight - r.top + 4 : undefined,
    });
  }, []);

  const open = useCallback(() => {
    if (disabled) return;
    measure();
    setVisibleCount(OPTION_PAGE_SIZE);
    setSearchQuery('');
    setActiveIndex(options.findIndex((opt) => opt.value === value));
    setIsOpen(true);
  }, [disabled, measure, options, value]);

  const close = useCallback((refocus = true) => {
    setIsOpen(false);
    setActiveIndex(-1);
    if (refocus) triggerRef.current?.focus();
  }, []);

  function commit(index: number) {
    const opt = filteredOptions[index];
    if (!opt) return;
    onChange(opt.value);
    close();
  }

  // Outside click closes; scroll and resize keep the panel on the trigger
  useEffect(() => {
    if (!isOpen) return;

    function handlePointerDown(event: MouseEvent) {
      const target = event.target as Node;
      if (containerRef.current?.contains(target)) return;
      if (panelRef.current?.contains(target)) return;
      close(false);
    }
    document.addEventListener('mousedown', handlePointerDown);
    window.addEventListener('scroll', measure, true);
    window.addEventListener('resize', measure);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      window.removeEventListener('scroll', measure, true);
      window.removeEventListener('resize', measure);
    };
  }, [isOpen, close, measure]);

  // Focus the search box when there is one, else the list itself
  useEffect(() => {
    if (!isOpen) return;
    const target = searchable ? searchInputRef.current : listRef.current;
    target?.focus();
  }, [isOpen, searchable]);

  useEffect(() => {
    if (!isOpen || activeIndex < 0) return;
    optionRefs.current[activeIndex]?.scrollIntoView({ block: 'nearest' });
  }, [isOpen, activeIndex]);

  function move(delta: number) {
    if (filteredOptions.length === 0) return;
    let next = activeIndex + delta;
    if (next < 0) next = filteredOptions.length - 1;
    if (next >= filteredOptions.length) next = 0;
    setActiveIndex(next);
    // The target may not be rendered yet under infinite scroll
    setVisibleCount((count) => Math.max(count, next + 1));
  }

  /** Jump to the next option starting with what was typed, like a native select */
  function jumpTo(char: string) {
    const now = Date.now();
    const text = now - typeahead.current.at < 700 ? typeahead.current.text + char : char;
    typeahead.current = { text, at: now };

    const from = Math.max(activeIndex + (text.length === 1 ? 1 : 0), 0);
    const ordered = [...filteredOptions.slice(from), ...filteredOptions.slice(0, from)];
    const hit = ordered.find((opt) => opt.label.toLowerCase().startsWith(text.toLowerCase()));
    if (!hit) return;

    const index = filteredOptions.indexOf(hit);
    setActiveIndex(index);
    setVisibleCount((count) => Math.max(count, index + 1));
  }

  function isTypeaheadKey(e: React.KeyboardEvent) {
    return !searchable && e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey;
  }

  function handleTriggerKeyDown(e: React.KeyboardEvent) {
    if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
      e.preventDefault();
      open();
      return;
    }
    if (isTypeaheadKey(e)) {
      e.preventDefault();
      open();
      jumpTo(e.key);
    }
  }

  function handlePanelKeyDown(e: React.KeyboardEvent) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        move(1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        move(-1);
        break;
      case 'Home':
        e.preventDefault();
        setActiveIndex(0);
        break;
      case 'End':
        e.preventDefault();
        setActiveIndex(filteredOptions.length - 1);
        setVisibleCount(filteredOptions.length);
        break;
      case 'Enter':
        e.preventDefault();
        commit(activeIndex);
        break;
      case 'Escape':
        e.preventDefault();
        close();
        break;
      case 'Tab':
        close(false);
        break;
      default:
        if (isTypeaheadKey(e)) {
          e.preventDefault();
          jumpTo(e.key);
        }
    }
  }

  const shell = ['ui-select', size === 'sm' && 'ui-select--sm'].filter(Boolean).join(' ');
  const activeId = activeIndex >= 0 ? `${baseId}-opt-${activeIndex}` : undefined;

  const panel = isOpen && pos && (
    <div
      ref={panelRef}
      className="ui-select__panel"
      style={{ left: pos.left, width: pos.width, top: pos.top, bottom: pos.bottom }}
    >
      {searchable && (
        <div className="flex items-center gap-2 border-b border-rule p-2">
          <Search className="h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />
          <input
            ref={searchInputRef}
            type="text"
            value={searchQuery}
            onChange={(e) => {
              setSearchQuery(e.target.value);
              setVisibleCount(OPTION_PAGE_SIZE);
              setActiveIndex(0);
            }}
            onKeyDown={handlePanelKeyDown}
            placeholder={searchPlaceholder}
            aria-controls={listId}
            aria-activedescendant={activeId}
            className="w-full bg-transparent text-sm text-ink placeholder:text-muted outline-none"
          />
          {searchQuery && (
            <button
              type="button"
              aria-label="Clear search"
              onClick={() => {
                setSearchQuery('');
                setVisibleCount(OPTION_PAGE_SIZE);
                searchInputRef.current?.focus();
              }}
              className="text-muted hover:text-ink"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      )}

      <div
        ref={listRef}
        id={listId}
        role="listbox"
        tabIndex={-1}
        aria-labelledby={label ? labelId : undefined}
        aria-activedescendant={activeId}
        onKeyDown={searchable ? undefined : handlePanelKeyDown}
        onScroll={handleOptionsScroll}
        className="max-h-56 overflow-y-auto p-1 text-sm outline-none"
      >
        {filteredOptions.length === 0 ? (
          <div className="py-3 text-center text-xs text-muted">No matching options found</div>
        ) : (
          <>
            {filteredOptions.slice(0, visibleCount).map((opt, index) => {
              const isSelected = opt.value === value;
              const isActive = index === activeIndex;
              return (
                <div
                  key={opt.value}
                  id={`${baseId}-opt-${index}`}
                  ref={(el) => {
                    optionRefs.current[index] = el;
                  }}
                  role="option"
                  aria-selected={isSelected}
                  onClick={() => commit(index)}
                  onMouseEnter={() => setActiveIndex(index)}
                  className={`flex w-full cursor-pointer items-center justify-between rounded-control px-2.5 py-1.5 text-left transition-colors ${
                    isSelected
                      ? 'bg-accent-wash font-semibold text-accent'
                      : `text-ink ${isActive ? 'bg-paper-2' : ''}`
                  }`}
                >
                  <div className="min-w-0 flex-1 truncate">
                    <div className="truncate font-medium">{opt.label}</div>
                    {opt.subtext && (
                      <div className="truncate font-mono text-2xs text-muted">{opt.subtext}</div>
                    )}
                  </div>
                  {isSelected && <Check className="ml-2 h-4 w-4 shrink-0 text-accent" />}
                </div>
              );
            })}
            {visibleCount < filteredOptions.length && (
              <div className="py-1.5 text-center text-2xs text-muted">Scroll for more…</div>
            )}
          </>
        )}
      </div>
    </div>
  );

  return (
    <div className={className} ref={containerRef}>
      {label && (
        <span className="ui-field__label" id={labelId}>
          {label}
          {required && (
            <span className="ui-field__req" aria-hidden="true">
              *
            </span>
          )}
        </span>
      )}

      <div className={shell}>
        <button
          ref={triggerRef}
          type="button"
          disabled={disabled}
          role="combobox"
          aria-haspopup="listbox"
          aria-expanded={isOpen}
          aria-controls={isOpen ? listId : undefined}
          aria-labelledby={label ? labelId : undefined}
          aria-label={ariaLabel}
          aria-required={required || undefined}
          onClick={() => (isOpen ? close() : open())}
          onKeyDown={handleTriggerKeyDown}
          className={`ui-input ui-select__control flex w-full items-center text-left text-ink ${controlClassName}`}
        >
          <span className="min-w-0 truncate">
            {selectedOption ? (
              <span className="flex items-center gap-1.5">
                <span className="font-semibold text-ink">{selectedOption.label}</span>
                {selectedOption.subtext && (
                  <span className="font-mono text-2xs text-muted">({selectedOption.subtext})</span>
                )}
              </span>
            ) : (
              <span className="text-muted">{placeholder}</span>
            )}
          </span>
        </button>
        <ChevronDown className="ui-select__chevron" aria-hidden="true" />

        {/* A button never blocks submit, so the browser validates this instead.
         * It sits under the control so the bubble points at the right field. */}
        {required && (
          <input
            className="ui-select__validity"
            tabIndex={-1}
            aria-hidden="true"
            required
            value={value}
            onChange={() => {}}
          />
        )}
      </div>

      {panel ? createPortal(panel, document.body) : null}
    </div>
  );
}

export type SearchableSelectProps = Omit<SelectProps, 'searchable'>;

/** Same dropdown with the search box switched on */
export function SearchableSelect(props: SearchableSelectProps) {
  return <Select {...props} searchable />;
}
