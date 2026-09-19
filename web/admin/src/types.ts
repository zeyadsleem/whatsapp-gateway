import { z } from 'zod';

export const instanceStatusSchema = z.enum([
  'disconnected',
  'connecting',
  'qr_required',
  'connected',
  'reconnecting',
  'logged_out',
  'error',
]);

export type InstanceStatus = z.infer<typeof instanceStatusSchema>;

export const instanceSchema = z.object({
  id: z.string(),
  shop_id: z.string(),
  display_name: z.string(),
  phone_number: z.string().nullable(),
  status: instanceStatusSchema,
  last_error: z.string().optional(),
  webhook_url: z.string().nullable(),
  webhook_enabled: z.boolean(),
  api_key_prefix: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
  last_connected_at: z.string().nullable(),
});

export type Instance = z.infer<typeof instanceSchema>;

export const statsSchema = z.object({
  total: z.number(),
  by_status: z.record(z.string(), z.number()),
  sent_ok: z.number(),
  sent_failed: z.number(),
  received: z.number(),
});

export type AdminStats = z.infer<typeof statsSchema>;

export const createdInstanceSchema = z.object({
  id: z.string(),
  shop_id: z.string(),
  display_name: z.string(),
  status: instanceStatusSchema,
  webhook_url: z.string().nullable(),
  webhook_enabled: z.boolean(),
  created_at: z.string(),
  api_key: z.string(),
  webhook_secret: z.string().optional(),
});

export type CreatedInstance = z.infer<typeof createdInstanceSchema>;

export const updateResultSchema = z.object({
  instance: instanceSchema,
  webhook_secret: z.string().optional(),
});

export type UpdateResult = z.infer<typeof updateResultSchema>;

export const qrStateSchema = z.object({
  status: z.string(),
  connected: z.boolean(),
  qr_code: z.string().optional(),
  qr_png_base64: z.string().optional(),
  expires_at: z.string().optional(),
});

export type QrState = z.infer<typeof qrStateSchema>;

export const instancesResponseSchema = z.object({
  instances: z.array(instanceSchema),
});

export const apiKeyResponseSchema = z.object({
  api_key: z.string(),
});

export interface CreateInstanceInput {
  shop_id: string;
  display_name: string;
  webhook_url: string;
}

export interface UpdateInstanceInput {
  display_name?: string;
  webhook_url?: string;
  webhook_enabled?: boolean;
  rotate_webhook_secret?: boolean;
}

export interface UpdateWebhookInput {
  webhook_url: string;
  webhook_enabled: boolean;
  rotate_webhook_secret?: boolean;
}

export const httpsUrlSchema = z
  .string()
  .trim()
  .refine((value) => {
    if (value === '') {
      return true;
    }
    try {
      return new URL(value).protocol === 'https:';
    } catch {
      return false;
    }
  }, 'يجب أن يكون رابطاً صحيحاً يبدأ بـ https://');

export function fieldErrors(error: z.ZodError): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const issue of error.issues) {
    const key = issue.path[0];
    if (typeof key === 'string' && errors[key] === undefined) {
      errors[key] = issue.message;
    }
  }
  return errors;
}
