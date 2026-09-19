import type { ReactElement } from 'react';

interface ErrorBannerProps {
  message: string;
  onRetry?: () => void;
}

export function ErrorBanner({ message, onRetry }: ErrorBannerProps): ReactElement {
  return (
    <div className="error-banner" role="alert">
      <span>{message}</span>
      {onRetry !== undefined ? (
        <button type="button" className="btn btn-secondary btn-sm" onClick={onRetry}>
          إعادة المحاولة
        </button>
      ) : null}
    </div>
  );
}
