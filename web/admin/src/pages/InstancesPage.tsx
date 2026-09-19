import { useState, type ReactElement } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  connectInstance,
  deleteInstance,
  disconnectInstance,
  fetchInstances,
  logoutInstance,
  restartInstance,
  rotateApiKey,
} from '../api';
import { ActionMenu, type ActionMenuItem } from '../components/ActionMenu';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { EmptyState } from '../components/EmptyState';
import { ErrorBanner } from '../components/ErrorBanner';
import { InstanceFormModal } from '../components/InstanceFormModal';
import { InstanceSettingsModal } from '../components/InstanceSettingsModal';
import { Layout } from '../components/Layout';
import { SecretsModal, type SecretField } from '../components/SecretsModal';
import { Spinner } from '../components/Spinner';
import { StatusBadge } from '../components/StatusBadge';
import { usePolling } from '../hooks';
import type { CreatedInstance, Instance } from '../types';

interface SecretsState {
  title: string;
  fields: SecretField[];
  pairInstanceId: string | null;
}

const dateFormatter = new Intl.DateTimeFormat('ar-EG-u-nu-latn', { dateStyle: 'medium', timeStyle: 'short' });

function formatDateTime(value: string | null): string {
  if (value === null) {
    return '—';
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : dateFormatter.format(date);
}

export function InstancesPage(): ReactElement {
  const navigate = useNavigate();
  const { data, error, loading, refresh } = usePolling(fetchInstances, 5000);
  const [formOpen, setFormOpen] = useState(false);
  const [secrets, setSecrets] = useState<SecretsState | null>(null);
  const [settingsInstance, setSettingsInstance] = useState<Instance | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Instance | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const runAction = async (instance: Instance, action: () => Promise<void>): Promise<void> => {
    setBusyId(instance.id);
    setActionError(null);
    try {
      await action();
      await refresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'تعذّر تنفيذ الإجراء');
    } finally {
      setBusyId(null);
    }
  };

  const handleRotate = async (instance: Instance): Promise<void> => {
    setBusyId(instance.id);
    setActionError(null);
    try {
      const apiKey = await rotateApiKey(instance.id);
      setSecrets({
        title: `مفتاح API الجديد لـ ${instance.display_name}`,
        fields: [{ label: 'مفتاح API', value: apiKey }],
        pairInstanceId: null,
      });
      await refresh();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'تعذّر تدوير المفتاح');
    } finally {
      setBusyId(null);
    }
  };

  const handleCreated = (created: CreatedInstance): void => {
    setFormOpen(false);
    const fields: SecretField[] = [{ label: 'مفتاح API', value: created.api_key }];
    if (created.webhook_secret !== undefined && created.webhook_secret !== '') {
      fields.push({ label: 'سر الويب هوك', value: created.webhook_secret });
    }
    setSecrets({
      title: 'بيانات الاتصال الجديدة',
      fields,
      pairInstanceId: created.id,
    });
    void refresh();
  };

  const handleDelete = async (): Promise<void> => {
    const target = deleteTarget;
    if (target === null) {
      return;
    }
    await runAction(target, () => deleteInstance(target.id));
    setDeleteTarget(null);
  };

  const buildItems = (instance: Instance): ActionMenuItem[] => {
    const items: ActionMenuItem[] = [
      {
        label: 'عرض QR',
        onClick: () => {
          void navigate(`/instances/${instance.id}/pair`);
        },
      },
    ];
    if (instance.status === 'disconnected' || instance.status === 'error') {
      items.push({
        label: 'اتصال',
        onClick: () => {
          void runAction(instance, () => connectInstance(instance.id));
        },
      });
    }
    items.push({
      label: 'إعادة تشغيل',
      onClick: () => {
        void runAction(instance, () => restartInstance(instance.id));
      },
    });
    if (instance.status === 'connected') {
      items.push({
        label: 'فصل',
        onClick: () => {
          void runAction(instance, () => disconnectInstance(instance.id));
        },
      });
    }
    items.push({
      label: 'تسجيل خروج',
      onClick: () => {
        void runAction(instance, () => logoutInstance(instance.id));
      },
    });
    items.push({ label: 'الإعدادات', onClick: () => setSettingsInstance(instance) });
    items.push({
      label: 'تدوير مفتاح API',
      onClick: () => {
        void handleRotate(instance);
      },
    });
    items.push({ label: 'حذف', danger: true, onClick: () => setDeleteTarget(instance) });
    return items;
  };

  return (
    <Layout title="الاتصالات">
      <div className="page-actions">
        <button type="button" className="btn btn-primary" onClick={() => setFormOpen(true)}>
          إضافة اتصال واتساب
        </button>
      </div>
      {actionError !== null ? <ErrorBanner message={actionError} /> : null}
      {error !== null ? <ErrorBanner message={error} onRetry={() => void refresh()} /> : null}
      {data === null ? (
        loading ? (
          <div className="loading-block">
            <Spinner />
          </div>
        ) : null
      ) : data.length === 0 ? (
        <EmptyState
          title="لا توجد اتصالات بعد"
          description="أضف أول اتصال واتساب لبدء الربط."
          action={
            <button type="button" className="btn btn-primary" onClick={() => setFormOpen(true)}>
              إضافة اتصال واتساب
            </button>
          }
        />
      ) : (
        <div className="table-wrap">
          <table className="instances-table">
            <thead>
              <tr>
                <th scope="col">اسم المحل</th>
                <th scope="col">shop_id</th>
                <th scope="col">رقم واتساب</th>
                <th scope="col">الحالة</th>
                <th scope="col">آخر اتصال</th>
                <th scope="col">تاريخ الإنشاء</th>
                <th scope="col">إجراءات</th>
              </tr>
            </thead>
            <tbody>
              {data.map((instance) => (
                <tr key={instance.id} className={busyId === instance.id ? 'row-busy' : undefined}>
                  <td>{instance.display_name}</td>
                  <td>
                    <code className="mono">{instance.shop_id}</code>
                  </td>
                  <td>
                    {instance.phone_number !== null ? (
                      <span className="mono">{instance.phone_number}</span>
                    ) : (
                      <span className="cell-muted">—</span>
                    )}
                  </td>
                  <td>
                    <StatusBadge status={instance.status} />
                  </td>
                  <td className="cell-muted">{formatDateTime(instance.last_connected_at)}</td>
                  <td className="cell-muted">{formatDateTime(instance.created_at)}</td>
                  <td>
                    <ActionMenu label={`إجراءات ${instance.display_name}`} items={buildItems(instance)} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {formOpen ? <InstanceFormModal onClose={() => setFormOpen(false)} onCreated={handleCreated} /> : null}

      {settingsInstance !== null ? (
        <InstanceSettingsModal
          instance={settingsInstance}
          onClose={() => setSettingsInstance(null)}
          onSaved={() => {
            setSettingsInstance(null);
            void refresh();
          }}
        />
      ) : null}

      {secrets !== null ? (
        <SecretsModal
          title={secrets.title}
          secrets={secrets.fields}
          onClose={() => setSecrets(null)}
          footer={
            secrets.pairInstanceId !== null ? (
              <>
                <button type="button" className="btn btn-secondary" onClick={() => setSecrets(null)}>
                  إغلاق
                </button>
                <button
                  type="button"
                  className="btn btn-primary"
                  onClick={() => {
                    const id = secrets.pairInstanceId;
                    setSecrets(null);
                    if (id !== null) {
                      void navigate(`/instances/${id}/pair`);
                    }
                  }}
                >
                  الانتقال إلى الربط
                </button>
              </>
            ) : undefined
          }
        />
      ) : null}

      {deleteTarget !== null ? (
        <ConfirmDialog
          title="حذف الاتصال"
          message={`سيتم حذف الاتصال "${deleteTarget.display_name}" وجلسة واتساب المرتبطة به نهائياً. لا يمكن التراجع عن هذا الإجراء.`}
          confirmLabel="حذف نهائي"
          busy={busyId === deleteTarget.id}
          onConfirm={() => void handleDelete()}
          onCancel={() => setDeleteTarget(null)}
        />
      ) : null}
    </Layout>
  );
}
