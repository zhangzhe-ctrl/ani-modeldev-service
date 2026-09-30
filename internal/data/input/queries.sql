-- name: InsertFrozenImport :one
INSERT INTO modeldev_input_versions (
    tenant_id, input_version_id, request_id, actor, requested_at,
    storage_connection_id, bucket, approved_prefix, credential_reference, object_key,
    object_version_id, size_bytes, sha256
) VALUES (
    sqlc.arg(tenant_id)::uuid, sqlc.arg(input_version_id)::uuid,
    sqlc.arg(request_id)::uuid, sqlc.arg(actor), sqlc.arg(requested_at),
    sqlc.arg(storage_connection_id), sqlc.arg(bucket), sqlc.arg(approved_prefix),
    sqlc.arg(credential_reference), sqlc.arg(object_key), sqlc.arg(object_version_id), sqlc.arg(size_bytes), sqlc.arg(sha256)
)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetInputVersion :one
SELECT * FROM modeldev_input_versions
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND input_version_id = sqlc.arg(input_version_id)::uuid;
