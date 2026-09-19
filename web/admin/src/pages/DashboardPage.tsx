import type { ReactElement } from 'react';
import { fetchStats } from '../api';
import { EmptyState } from '../components/EmptyState';
import { ErrorBanner } from '../components/ErrorBanner';
import { Layout } from '../components/Layout';
import { Spinner } from '../components/Spinner';
import { StatusBadge } from '../components/StatusBadge';
import { usePolling } from '../hooks';
import type { AdminStats } from '../types';

interface StatCard {
  label: string;
  value: number;
  tone: string;
}

function countOf(stats: AdminStats, status: string): number {
  return stats.by_status[status] ?? 0;
}

function buildCards(stats: AdminStats): StatCard[] {
  return [
    { label: 'إجمالي الاتصالات', value: stats.total, tone: 'neutral' },
    { label: 'متصلة', value: countOf(stats, 'connected'), tone: 'success' },
    { label: 'غير متصلة', value: Math.max(0, stats.total - countOf(stats, 'connected')), tone: 'muted' },
    { label: 'تحتاج ربط', value: countOf(stats, 'qr_required'), tone: 'warning' },
    { label: 'أخطاء', value: countOf(stats, 'error'), tone: 'danger' },
    { label: 'مسجّل خروج', value: countOf(stats, 'logged_out'), tone: 'rose' },
    { label: 'مرسلة بنجاح', value: stats.sent_ok, tone: 'success' },
    { label: 'فشل الإرسال', value: stats.sent_failed, tone: 'danger' },
    { label: 'رسائل واردة', value: stats.received, tone: 'info' },
  ];
}

export function DashboardPage(): ReactElement {
  const { data, error, loading, refresh } = usePolling(fetchStats, 5000);

  return (
    <Layout title="لوحة التحكم">
      {error !== null ? <ErrorBanner message={error} onRetry={() => void refresh()} /> : null}
      {data === null ? (
        loading ? (
          <div className="loading-block">
            <Spinner />
          </div>
        ) : null
      ) : (
        <>
          <section className="stats-grid" aria-label="ملخص الاتصالات">
            {buildCards(data).map((card) => (
              <article key={card.label} className={`stat-card stat-${card.tone}`}>
                <p className="stat-card-label">{card.label}</p>
                <p className="stat-card-value">{card.value}</p>
              </article>
            ))}
          </section>
          <section className="panel breakdown-panel" aria-label="توزيع الحالات">
            <header className="panel-header">توزيع الحالات</header>
            {Object.keys(data.by_status).length === 0 ? (
              <div className="panel-body">
                <EmptyState title="لا توجد بيانات" />
              </div>
            ) : (
              <ul className="breakdown-list">
                {Object.entries(data.by_status)
                  .sort((first, second) => second[1] - first[1])
                  .map(([status, count]) => (
                    <li key={status} className="breakdown-item">
                      <StatusBadge status={status} />
                      <span className="breakdown-count">{count}</span>
                    </li>
                  ))}
              </ul>
            )}
          </section>
        </>
      )}
    </Layout>
  );
}
