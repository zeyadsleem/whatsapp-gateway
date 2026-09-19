import { useState, type FormEvent, type ReactElement } from 'react';
import { useNavigate } from 'react-router-dom';
import { login } from '../api';
import { ErrorBanner } from '../components/ErrorBanner';

export function LoginPage(): ReactElement {
  const navigate = useNavigate();
  const [apiKey, setApiKey] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    const key = apiKey.trim();
    if (key === '') {
      setError('يرجى إدخال مفتاح الإدارة');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await login(key);
      void navigate('/', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'تعذّر تسجيل الدخول');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login-page">
      <form className="login-card" onSubmit={(event) => void handleSubmit(event)} noValidate>
        <div className="brand login-brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
            </svg>
          </span>
          <span className="brand-text">بوابة واتساب</span>
        </div>
        <h1>لوحة الإدارة</h1>
        <p className="login-hint">أدخل مفتاح الإدارة للوصول إلى اللوحة.</p>
        {error !== null ? <ErrorBanner message={error} /> : null}
        <div className="field">
          <label htmlFor="admin-key">مفتاح الإدارة</label>
          <input
            id="admin-key"
            className="input"
            type="password"
            dir="ltr"
            value={apiKey}
            autoFocus
            autoComplete="off"
            onChange={(event) => setApiKey(event.target.value)}
          />
        </div>
        <button type="submit" className="btn btn-primary btn-block" disabled={busy}>
          {busy ? 'جارٍ التحقق...' : 'دخول'}
        </button>
      </form>
    </div>
  );
}
