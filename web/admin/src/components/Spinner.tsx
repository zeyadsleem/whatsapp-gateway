import type { ReactElement } from 'react';

interface SpinnerProps {
  label?: string;
}

export function Spinner({ label = 'جارٍ التحميل' }: SpinnerProps): ReactElement {
  return (
    <span className="spinner-wrap" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" />
      <span className="visually-hidden">{label}</span>
    </span>
  );
}
