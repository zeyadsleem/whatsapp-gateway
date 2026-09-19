import { useState, type FormEvent, type ReactElement } from 'react';
import { z } from 'zod';
import { createInstance } from '../api';
import { fieldErrors, httpsUrlSchema, type CreatedInstance, type CreateInstanceInput } from '../types';
import { ErrorBanner } from './ErrorBanner';
import { Modal } from './Modal';

const formSchema = z.object({
  shop_id: z
    .string()
    .trim()
    .regex(/^[A-Za-z0-9._-]{1,64}$/, 'معرّف المحل يقبل الأحرف الإنجليزية والأرقام والنقاط والشرطات فقط، بحد أقصى 64 حرفاً'),
  display_name: z.string().trim().min(1, 'اسم المحل مطلوب').max(128, 'اسم المحل طويل جداً'),
  webhook_url: httpsUrlSchema,
});

interface InstanceFormModalProps {
  onClose: () => void;
  onCreated: (instance: CreatedInstance) => void;
}

export function InstanceFormModal({ onClose, onCreated }: InstanceFormModalProps): ReactElement {
  const [shopId, setShopId] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [webhookUrl, setWebhookUrl] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    const parsed = formSchema.safeParse({ shop_id: shopId, display_name: displayName, webhook_url: webhookUrl });
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    setSubmitError(null);
    setBusy(true);
    const input: CreateInstanceInput = {
      shop_id: parsed.data.shop_id,
      display_name: parsed.data.display_name,
      webhook_url: parsed.data.webhook_url,
    };
    try {
      const created = await createInstance(input);
      onCreated(created);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'تعذّر إنشاء الاتصال');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      title="إضافة اتصال واتساب"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="btn btn-secondary" onClick={onClose} disabled={busy}>
            إلغاء
          </button>
          <button type="submit" form="instance-form" className="btn btn-primary" disabled={busy}>
            {busy ? 'جارٍ الإنشاء...' : 'إنشاء'}
          </button>
        </>
      }
    >
      {submitError !== null ? <ErrorBanner message={submitError} /> : null}
      <form id="instance-form" className="form" onSubmit={(event) => void handleSubmit(event)} noValidate>
        <div className="field">
          <label htmlFor="shop-id">shop_id</label>
          <input
            id="shop-id"
            className="input"
            type="text"
            dir="ltr"
            value={shopId}
            autoFocus
            onChange={(event) => setShopId(event.target.value)}
            aria-invalid={errors.shop_id !== undefined}
          />
          {errors.shop_id !== undefined ? <p className="field-error">{errors.shop_id}</p> : null}
        </div>
        <div className="field">
          <label htmlFor="display-name">اسم المحل</label>
          <input
            id="display-name"
            className="input"
            type="text"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            aria-invalid={errors.display_name !== undefined}
          />
          {errors.display_name !== undefined ? <p className="field-error">{errors.display_name}</p> : null}
        </div>
        <div className="field">
          <label htmlFor="webhook-url">رابط Webhook (اختياري)</label>
          <input
            id="webhook-url"
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
      </form>
    </Modal>
  );
}
