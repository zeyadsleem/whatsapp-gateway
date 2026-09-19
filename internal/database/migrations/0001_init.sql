CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE app.instances (
    id                 uuid PRIMARY KEY,
    shop_id            text NOT NULL UNIQUE,
    display_name       text NOT NULL,
    wa_jid             text,
    phone_number       text,
    status             text NOT NULL DEFAULT 'disconnected',
    webhook_url        text,
    webhook_secret_enc bytea,
    webhook_enabled    boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    last_connected_at  timestamptz,
    CONSTRAINT instances_shop_id_format CHECK (shop_id ~ '^[A-Za-z0-9._-]{1,64}$'),
    CONSTRAINT instances_status_valid CHECK (
        status IN ('disconnected','connecting','qr_required','connected','reconnecting','logged_out','error')
    )
);

CREATE INDEX instances_status_idx ON app.instances (status);

CREATE TABLE app.api_keys (
    id           uuid PRIMARY KEY,
    instance_id  uuid NOT NULL REFERENCES app.instances(id) ON DELETE CASCADE,
    key_hash     text NOT NULL UNIQUE,
    key_prefix   text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE UNIQUE INDEX api_keys_instance_uidx ON app.api_keys (instance_id);
