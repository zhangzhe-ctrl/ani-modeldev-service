-- A frozen import is not READY. Remote reads happen after this commit and
-- outside database transactions, always against this exact object version.
CREATE TABLE modeldev_input_versions (
    tenant_id uuid NOT NULL CHECK (tenant_id <> '00000000-0000-0000-0000-000000000000'),
    input_version_id uuid NOT NULL CHECK (input_version_id <> '00000000-0000-0000-0000-000000000000'),
    request_id uuid NOT NULL CHECK (request_id <> '00000000-0000-0000-0000-000000000000'),
    actor text NOT NULL CHECK (actor <> ''),
    requested_at timestamptz NOT NULL,
    storage_connection_id text NOT NULL CHECK (storage_connection_id <> ''),
    bucket text NOT NULL CHECK (bucket <> ''),
    approved_prefix text NOT NULL CHECK (approved_prefix <> ''),
    object_key text NOT NULL CHECK (object_key <> ''),
    object_version_id text NOT NULL CHECK (object_version_id <> '' AND lower(object_version_id) <> 'null'),
    size_bytes bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 33554432),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'VALIDATING' CHECK (state = 'VALIDATING'),
    PRIMARY KEY (tenant_id, input_version_id),
    UNIQUE (tenant_id, request_id)
);

ALTER TABLE modeldev_input_versions DISABLE ROW LEVEL SECURITY;
