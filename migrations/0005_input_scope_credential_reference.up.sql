-- A reference identifies owner-managed credentials; it never contains secrets.
-- Preserve it with the frozen scope, including the supported empty reference.
ALTER TABLE modeldev_input_versions
    ADD COLUMN credential_reference text NOT NULL DEFAULT '';
