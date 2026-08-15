'use client';

import React, { useState, useRef, useEffect } from 'react';
import { Search, ChevronDown, Check, X } from 'lucide-react';

export interface SelectOption {
  value: string;
  label: string;
  subtext?: string;
  badge?: string;
}

export interface SearchableSelectProps {
  label?: string;
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  searchPlaceholder?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
}

export function SearchableSelect({
  label,
  value,
  onChange,
  options = [],
  placeholder = 'Choose...',
  searchPlaceholder = 'Search...',
  disabled = false,
  className = '',
}: SearchableSelectProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const selectedOption = options.find((opt) => opt.value === value);

  const filteredOptions = options.filter((opt) => {
    if (!searchQuery.trim()) return true;
    const query = searchQuery.toLowerCase();
    const matchLabel = opt.label.toLowerCase().includes(query);
    const matchSubtext = opt.subtext?.toLowerCase().includes(query);
    const matchBadge = opt.badge?.toLowerCase().includes(query);
    return matchLabel || matchSubtext || matchBadge;
  });

  // Close on outside click
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen]);

  // Focus search input on open
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => {
        searchInputRef.current?.focus();
      }, 50);
    } else {
      setSearchQuery('');
    }
  }, [isOpen]);

  return (
    <div className={`relative ${className}`} ref={containerRef}>
      {label && <span className="ui-field__label block mb-1">{label}</span>}

      <button
        type="button"
        disabled={disabled}
        onClick={() => setIsOpen(!isOpen)}
        className="ui-input flex w-full items-center justify-between gap-2 text-left text-xs text-ink cursor-pointer focus:border-focus"
      >
        <span className="truncate">
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
        <ChevronDown className="h-4 w-4 shrink-0 text-muted" />
      </button>

      {isOpen && (
        <div className="absolute z-50 mt-1 w-full rounded-surface border border-rule bg-paper shadow-lg">
          {/* Search Box */}
          <div className="flex items-center gap-2 border-b border-rule p-2">
            <Search className="h-3.5 w-3.5 shrink-0 text-muted" />
            <input
              ref={searchInputRef}
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={searchPlaceholder}
              className="w-full bg-transparent text-xs text-ink placeholder:text-muted outline-none"
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => setSearchQuery('')}
                className="text-muted hover:text-ink"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>

          {/* Options List */}
          <div className="max-h-56 overflow-y-auto p-1 text-xs">
            {filteredOptions.length === 0 ? (
              <div className="py-3 text-center text-xs text-muted">
                No matching options found
              </div>
            ) : (
              filteredOptions.map((opt) => {
                const isSelected = opt.value === value;
                return (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => {
                      onChange(opt.value);
                      setIsOpen(false);
                    }}
                    className={`flex w-full items-center justify-between rounded-control px-2.5 py-1.5 text-left transition-colors ${
                      isSelected
                        ? 'bg-accent-wash font-semibold text-accent'
                        : 'text-ink hover:bg-paper-2'
                    }`}
                  >
                    <div className="min-w-0 flex-1 truncate">
                      <div className="truncate font-medium">{opt.label}</div>
                      {opt.subtext && (
                        <div className="truncate font-mono text-[10px] text-muted">
                          {opt.subtext}
                        </div>
                      )}
                    </div>
                    {isSelected && <Check className="h-4 w-4 shrink-0 text-accent ml-2" />}
                  </button>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
