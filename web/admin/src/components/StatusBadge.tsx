import type { ReactElement } from 'react';
import type { InstanceStatus } from '../types';

const STATUS_LABELS: Record<InstanceStatus, string> = {
  connected: 'متصل',
  disconnected: 'غير متصل',
  connecting: 'جاري الاتصال',
  qr_required: 'يتطلب QR',
  reconnecting: 'إعادة الاتصال',
  logged_out: 'مسجّل خروج',
  error: 'خطأ',
};

const STATUS_TONES: Record<InstanceStatus, string> = {
  connected: 'success',
  disconnected: 'muted',
  connecting: 'info',
  qr_required: 'warning',
  reconnecting: 'info',
  logged_out: 'rose',
  error: 'danger',
};

export function statusLabel(status: string): string {
  return STATUS_LABELS[status as InstanceStatus] ?? status;
}

function statusTone(status: string): string {
  return STATUS_TONES[status as InstanceStatus] ?? 'muted';
}

interface StatusBadgeProps {
  status: string;
}

export function StatusBadge({ status }: StatusBadgeProps): ReactElement {
  return <span className={`status-badge status-${statusTone(status)}`}>{statusLabel(status)}</span>;
}
