import type { ReactElement, ReactNode } from 'react';
import { CopyField } from './CopyField';
import { Modal } from './Modal';

export interface SecretField {
  label: string;
  value: string;
}

interface SecretsModalProps {
  title: string;
  secrets: SecretField[];
  onClose: () => void;
  footer?: ReactNode;
}

export function SecretsModal({ title, secrets, onClose, footer }: SecretsModalProps): ReactElement {
  return (
    <Modal
      title={title}
      onClose={onClose}
      footer={
        footer ?? (
          <button type="button" className="btn btn-primary" onClick={onClose}>
            تم
          </button>
        )
      }
    >
      <p className="notice notice-warning">
        احفظ هذه القيم الآن داخل نظام المحل. لن تُعرض مرة أخرى بعد إغلاق هذه النافذة.
      </p>
      {secrets.map((secret) => (
        <CopyField key={secret.label} label={secret.label} value={secret.value} />
      ))}
    </Modal>
  );
}
