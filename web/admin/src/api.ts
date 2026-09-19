import { z } from 'zod';
import {
  apiKeyResponseSchema,
  createdInstanceSchema,
  instancesResponseSchema,
  instanceSchema,
  qrStateSchema,
  statsSchema,
  updateResultSchema,
  type AdminStats,
  type CreatedInstance,
  type CreateInstanceInput,
  type Instance,
  type QrState,
  type UpdateInstanceInput,
  type UpdateResult,
  type UpdateWebhookInput,
} from './types';

const API_BASE = '/api/v1/admin';
const ADMIN_KEY_STORAGE = 'wg_admin_key';

const ERROR_MESSAGES: Record<string, string> = {
  unauthorized: 'مفتاح غير صحيح',
  rate_limited: 'تم تجاوز الحد المسموح',
  instance_not_connected: 'الاتصال غير جاهز',
  not_found: 'العنصر المطلوب غير موجود',
  invalid_id: 'معرّف غير صالح',
  validation_error: 'البيانات المدخلة غير صالحة',
  body_too_large: 'حجم الطلب كبير جداً',
  conflict: 'تعارض في البيانات',
  network_error: 'تعذّر الاتصال بالخادم',
  invalid_response: 'استجابة غير صالحة من الخادم',
  internal_error: 'حدث خطأ في الخادم',
};

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

let unauthorizedHandler: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null): void {
  unauthorizedHandler = handler;
}

export function getAdminKey(): string | null {
  return sessionStorage.getItem(ADMIN_KEY_STORAGE);
}

export function setAdminKey(key: string): void {
  sessionStorage.setItem(ADMIN_KEY_STORAGE, key);
}

export function clearAdminKey(): void {
  sessionStorage.removeItem(ADMIN_KEY_STORAGE);
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  body?: unknown;
  apiKey?: string;
}

const errorBodySchema = z.object({
  error: z.object({ code: z.string(), message: z.string() }),
});

async function readErrorBody(response: Response): Promise<{ code: string; message: string } | null> {
  const text = await response.text();
  if (text.length === 0) {
    return null;
  }
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch {
    return null;
  }
  const parsed = errorBodySchema.safeParse(raw);
  return parsed.success ? parsed.data.error : null;
}

async function send(path: string, options: RequestOptions = {}): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  const apiKey = options.apiKey ?? getAdminKey();
  if (apiKey !== null) {
    headers.Authorization = `Bearer ${apiKey}`;
  }
  if (options.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method: options.method ?? 'GET',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch {
    throw new ApiError(0, 'network_error', ERROR_MESSAGES.network_error ?? 'تعذّر الاتصال بالخادم');
  }

  if (!response.ok) {
    if (response.status === 401) {
      clearAdminKey();
      unauthorizedHandler?.();
    }
    const error = await readErrorBody(response);
    const code = error?.code ?? `http_${response.status}`;
    const message = ERROR_MESSAGES[code] ?? error?.message ?? 'حدث خطأ غير متوقع';
    throw new ApiError(response.status, code, message);
  }

  return response;
}

async function readJson<T>(response: Response, schema: z.ZodType<T>): Promise<T> {
  let raw: unknown;
  try {
    raw = await response.json();
  } catch {
    throw new ApiError(response.status, 'invalid_response', ERROR_MESSAGES.invalid_response ?? 'استجابة غير صالحة من الخادم');
  }
  const parsed = schema.safeParse(raw);
  if (!parsed.success) {
    throw new ApiError(response.status, 'invalid_response', ERROR_MESSAGES.invalid_response ?? 'استجابة غير صالحة من الخادم');
  }
  return parsed.data;
}

function instancePath(id: string): string {
  return `/instances/${encodeURIComponent(id)}`;
}

export async function login(adminKey: string): Promise<AdminStats> {
  const response = await send('/stats', { apiKey: adminKey });
  const stats = await readJson(response, statsSchema);
  setAdminKey(adminKey);
  return stats;
}

export async function fetchStats(): Promise<AdminStats> {
  return readJson(await send('/stats'), statsSchema);
}

export async function fetchInstances(): Promise<Instance[]> {
  const data = await readJson(await send('/instances'), instancesResponseSchema);
  return data.instances;
}

export async function fetchInstance(id: string): Promise<Instance> {
  return readJson(await send(instancePath(id)), instanceSchema);
}

export async function createInstance(input: CreateInstanceInput): Promise<CreatedInstance> {
  return readJson(await send('/instances', { method: 'POST', body: input }), createdInstanceSchema);
}

export async function updateInstance(id: string, input: UpdateInstanceInput): Promise<UpdateResult> {
  return readJson(await send(instancePath(id), { method: 'PATCH', body: input }), updateResultSchema);
}

export async function updateWebhook(id: string, input: UpdateWebhookInput): Promise<UpdateResult> {
  return readJson(await send(`${instancePath(id)}/webhook`, { method: 'PUT', body: input }), updateResultSchema);
}

export async function deleteInstance(id: string): Promise<void> {
  await send(instancePath(id), { method: 'DELETE' });
}

export async function connectInstance(id: string): Promise<void> {
  await send(`${instancePath(id)}/connect`, { method: 'POST' });
}

export async function restartInstance(id: string): Promise<void> {
  await send(`${instancePath(id)}/restart`, { method: 'POST' });
}

export async function disconnectInstance(id: string): Promise<void> {
  await send(`${instancePath(id)}/disconnect`, { method: 'POST' });
}

export async function logoutInstance(id: string): Promise<void> {
  await send(`${instancePath(id)}/logout`, { method: 'POST' });
}

export async function rotateApiKey(id: string): Promise<string> {
  const data = await readJson(await send(`${instancePath(id)}/api-key/rotate`, { method: 'POST' }), apiKeyResponseSchema);
  return data.api_key;
}

export async function fetchQr(id: string): Promise<QrState> {
  return readJson(await send(`${instancePath(id)}/qr`), qrStateSchema);
}
