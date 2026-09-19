import { useEffect, useId, useRef, useState, type ReactElement } from 'react';

interface CopyFieldProps {
  label: string;
  value: string;
}

type CopyState = 'idle' | 'copied' | 'failed';

export function CopyField({ label, value }: CopyFieldProps): ReactElement {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const timeoutRef = useRef<number | null>(null);
  const [state, setState] = useState<CopyState>('idle');

  useEffect(
    () => () => {
      if (timeoutRef.current !== null) {
        window.clearTimeout(timeoutRef.current);
      }
    },
    [],
  );

  const resetLater = (): void => {
    if (timeoutRef.current !== null) {
      window.clearTimeout(timeoutRef.current);
    }
    timeoutRef.current = window.setTimeout(() => setState('idle'), 2000);
  };

  const handleCopy = async (): Promise<void> => {
    let copied = false;
    try {
      await navigator.clipboard.writeText(value);
      copied = true;
    } catch {
      const input = inputRef.current;
      if (input !== null) {
        input.select();
        copied = document.execCommand('copy');
      }
    }
    setState(copied ? 'copied' : 'failed');
    resetLater();
  };

  return (
    <div className="copy-field">
      <label className="copy-field-label" htmlFor={inputId}>
        {label}
      </label>
      <div className="copy-field-row">
        <input
          ref={inputRef}
          id={inputId}
          className="copy-field-input"
          value={value}
          readOnly
          dir="ltr"
          onFocus={(event) => event.currentTarget.select()}
        />
        <button type="button" className="btn btn-secondary" onClick={() => void handleCopy()}>
          {state === 'copied' ? 'تم النسخ' : 'نسخ'}
        </button>
      </div>
      {state === 'failed' ? (
        <p className="field-error" role="alert">
          تعذّر النسخ، انسخ القيمة يدوياً.
        </p>
      ) : null}
    </div>
  );
}
