import { useCallback, useEffect, useState, type ReactElement } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { ApiError, fetchInstance, fetchQr, restartInstance } from '../api';
import { ErrorBanner } from '../components/ErrorBanner';
import { Layout } from '../components/Layout';
import { Spinner } from '../components/Spinner';
import { StatusBadge } from '../components/StatusBadge';
import { usePolling } from '../hooks';
import type { Instance, QrState } from '../types';

interface PairData {
  instance: Instance;
  qr: QrState | null;
}

function secondsUntil(expiresAt: string | undefined, now: number): number | null {
  if (expiresAt === undefined) {
    return null;
  }
  const target = new Date(expiresAt).getTime();
  if (Number.isNaN(target)) {
    return null;
  }
  return Math.max(0, Math.ceil((target - now) / 1000));
}

function formatDuration(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60)
    .toString()
    .padStart(2, '0');
  const seconds = (totalSeconds % 60).toString().padStart(2, '0');
  return `${minutes}:${seconds}`;
}

export function PairPage(): ReactElement {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const [now, setNow] = useState(() => Date.now());
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(async (): Promise<PairData> => {
    const instance = await fetchInstance(id);
    let qr: QrState | null = null;
    try {
      qr = await fetchQr(id);
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 404)) {
        throw err;
      }
    }
    return { instance, qr };
  }, [id]);

  const { data, error, loading, refresh } = usePolling(load, 2000);

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const handleRestart = async (): Promise<void> => {
    setBusy(true);
    setActionError(null);
    try {
      await restartInstance(id);
      await refresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'تعذّر إعادة التشغيل');
    } finally {
      setBusy(false);
    }
  };

  const instance = data?.instance ?? null;
  const qr = data?.qr ?? null;
  const connected = qr?.connected === true;
  const remaining = qr !== null && !connected ? secondsUntil(qr.expires_at, now) : null;

  return (
    <Layout title="ربط واتساب">
      {actionError !== null ? <ErrorBanner message={actionError} /> : null}
      {data === null ? (
        error !== null ? (
          <>
            <ErrorBanner message={error} />
            <button type="button" className="btn btn-secondary" onClick={() => void navigate('/instances')}>
              العودة إلى القائمة
            </button>
          </>
        ) : loading ? (
          <div className="loading-block">
            <Spinner />
          </div>
        ) : null
      ) : connected && instance !== null ? (
        <section className="panel success-card">
          <span className="success-icon" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="20 6 9 17 4 12" />
            </svg>
          </span>
          <h2>تم ربط واتساب بنجاح</h2>
          <p className="success-phone">
            رقم واتساب: <span className="mono">{instance.phone_number ?? 'غير معروف'}</span>
          </p>
          <button type="button" className="btn btn-primary" onClick={() => void navigate('/instances')}>
            العودة إلى قائمة الاتصالات
          </button>
        </section>
      ) : (
        <section className="pair-grid">
          <div className="panel">
            <header className="panel-header">خطوات الربط</header>
            <div className="panel-body">
              <ol className="instructions">
                <li>افتح واتساب على هاتف صاحب المحل</li>
                <li>افتح الأجهزة المرتبطة</li>
                <li>اضغط ربط جهاز</li>
                <li>امسح رمز QR</li>
              </ol>
              {instance !== null ? (
                <div className="pair-status">
                  <span className="cell-muted">الحالة الحالية:</span>
                  <StatusBadge status={instance.status} />
                </div>
              ) : null}
              {instance?.last_error !== undefined && instance.last_error !== '' ? (
                <p className="pair-last-error">{instance.last_error}</p>
              ) : null}
            </div>
          </div>
          <div className="panel qr-card">
            <div className="panel-body">
              {qr?.qr_png_base64 !== undefined ? (
                <img className="qr-image" src={qr.qr_png_base64} alt="رمز QR" width={260} height={260} />
              ) : (
                <div className="qr-waiting">
                  <Spinner />
                  <p>جارٍ تجهيز رمز QR...</p>
                </div>
              )}
              {remaining !== null ? (
                <p className="countdown">
                  {remaining > 0 ? `ينتهي خلال ${formatDuration(remaining)}` : 'جارٍ تحديث الرمز...'}
                </p>
              ) : null}
              <div className="pair-actions">
                <button type="button" className="btn btn-secondary" onClick={() => void handleRestart()} disabled={busy}>
                  {busy ? 'جارٍ إعادة التشغيل...' : 'إعادة تشغيل'}
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => void navigate('/instances')}>
                  إلغاء
                </button>
              </div>
            </div>
          </div>
        </section>
      )}
    </Layout>
  );
}
