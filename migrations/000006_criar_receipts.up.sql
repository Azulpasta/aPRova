CREATE TABLE receipts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           bigint NOT NULL CHECK (tenant_id > 0),
    task_run_id         uuid NOT NULL UNIQUE REFERENCES task_runs (id) ON DELETE RESTRICT,
    previous_receipt_id uuid REFERENCES receipts (id) ON DELETE RESTRICT,
    payload             text NOT NULL,
    content_hash        bytea NOT NULL CHECK (octet_length(content_hash) = 32),
    signature           bytea NOT NULL CHECK (octet_length(signature) = 64),
    key_id              text NOT NULL,
    signed_at           timestamptz NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX receipts_tenant_id ON receipts (tenant_id);

-- A cadeia de recibos é linear: cada recibo tem no máximo um sucessor.
CREATE UNIQUE INDEX receipts_um_sucessor ON receipts (previous_receipt_id)
    WHERE previous_receipt_id IS NOT NULL;

CREATE FUNCTION receipts_recusar_alteracao() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'receipts é append-only: % não é permitido', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER receipts_append_only
    BEFORE UPDATE OR DELETE ON receipts
    FOR EACH ROW EXECUTE FUNCTION receipts_recusar_alteracao();

CREATE TRIGGER receipts_append_only_truncate
    BEFORE TRUNCATE ON receipts
    FOR EACH STATEMENT EXECUTE FUNCTION receipts_recusar_alteracao();
