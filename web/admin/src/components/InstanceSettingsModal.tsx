import { useState, type FormEvent, type ReactElement } from 'react';
import { z } from 'zod';
import { updateInstance } from '../api';
import { fieldErrors, httpsUrlSchema, type Instance } from '../types';
import { ErrorBanner } from './ErrorBanner';
import { Modal } from './Modal';

const formSchema = z.object({
  display_name: z.string().trim().min(1, 'اسم المحل مطلوب').max(128, 'اسم المحل طويل جداً'),
  webhook_url: httpsUrlSchema,
  webhook_enabled: z.boolean(),
});

interface InstanceSettingsModalProps {
  instance: Instance;
  onClose: () => void;
  onSaved: (instance: Instance) => void;
}

export function InstanceSettingsModal({ instance, onClose, onSaved }: InstanceSettingsModalProps): ReactElement {
  const [displayName, setDisplayName] = useState(instance.display_name);
  const [webhookUrl, setWebhookUrl] = useState(instance.webhook_url ?? '');
  const [webhookEnabled, setWebhookEnabled] = useState(instance.webhook_enabled);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    const parsed = formSchema.safeParse({
      display_name: displayName,
      webhook_url: webhookUrl,
      webhook_enabled: webhookEnabled,
    });
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    setSubmitError(null);
    setBusy(true);
    try {
      const result = await updateInstance(instance.id, {
        display_name: parsed.data.display_name,
        webhook_url: parsed.data.webhook_url,
        webhook_enabled: parsed.data.webhook_enabled,
      });
      onSaved(result.instance);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'تعذّر حفظ الإعدادات');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="إعدادات الاتصال"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={busy}>
            إلغاء
          </button>
          <button type="submit" form="instance-settings-form" className="btn btn-primary" disabled={busy}>
            {busy ? 'جارٍ الحفظ...' : 'حفظ'}
          </button>
        </>
      }
    >
      {submitError !== null ? <ErrorBanner message={submitError} /> : null}
      <form id="instance-settings-form" className="form" onSubmit={(event) => void handleSubmit(event)} noValidate>
        <div className="field">
          <label htmlFor="settings-shop-id">shop_id</label>
          <input id="settings-shop-id" className="input" type="text" dir="ltr" value={instance.shop_id} readOnly />
        </div>
        <div className="field">
          <label htmlFor="settings-display-name">اسم المحل</label>
          <input
            id="settings-display-name"
            className="input"
            type="text"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            aria-invalid={errors.display_name !== undefined}
          />
          {errors.display_name !== undefined ? <p className="field-error">{errors.display_name}</p> : null}
        </div>
        <div className="field">
          <label htmlFor="settings-webhook-url">رابط Webhook</label>
          <input
            id="settings-webhook-url"
            className="input"
            type="url"
            dir="ltr"
            value={webhookUrl}
            placeholder="https://example.com/webhook"
            onChange={(event) => setWebhookUrl(event.target.value)}
            aria-invalid={errors.webhook_url !== undefined}
          />
          {errors.webhook_url !== undefined ? <p className="field-error">{errors.webhook_url}</p> : null}
        </div>
        <div className="checkbox-field">
          <input
            id="settings-webhook-enabled"
            type="checkbox"
            checked={webhookEnabled}
            onChange={(event) => setWebhookEnabled(event.target.checked)}
          />
          <label htmlFor="settings-webhook-enabled">تفعيل إرسال الويب هوك</label>
        </div>
      </form>
    </Modal>
  );
}
