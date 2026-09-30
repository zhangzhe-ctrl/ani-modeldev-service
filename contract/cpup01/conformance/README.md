# CPU-P01 consumer conformance fixtures

This package is for consumer tests only. Its example UUIDs, registry.example.test
image, storage names, and repeated-byte digests are synthetic. Do not import
it from production code, use it as an environment default, or treat it as ENV
handoff evidence.

On the isolated Fedora checkout, run:

```sh
python3 scripts/generate-contract-vectors.py
python3 scripts/generate-contract-vectors.py --check
```

The generator extracts the manually specified raw/want literals from the
original intent and snapshot contract tests. It independently checks canonical
SHA256 with Python hashlib and writes three UTF-8 files without a trailing
newline. It never invokes the Go implementation to create expected values.
Preserve those original handwritten literals as the source of truth; replacing
them with these generated files would create a circular conformance check.

Return generated files to the workstation for review and commit, then verify
the new commit on Fedora. Missing embedded files are an expected build failure
until this generation step has completed; do not create placeholder JSON.

Consumers use IntentV1/SnapshotV1 for fresh typed values and compare their actual
wire round trips with the canonical byte accessors and fixed SHA256 constants.
Mutable slices returned to one test do not change the next test's fixture.

The Release v1 vector is maintained directly in `release-v1.json`. It was moved
byte-for-byte from the manually authored catalogue test literal on Fedora:
2362 bytes, SHA256 `388c7687614c92a2b50a14c8ee29df04f4d8934cf1dbf88d7b31258efc9f3dae`.
Its expected digest was independently computed with Python hashlib. The file has
no trailing newline; the generator above intentionally leaves it unchanged.
`ReleaseCanonicalV1`, `ReleaseV1` and `ReleaseSHA256V1` provide the shared consumer
test inputs. The catalogue test retains an independently authored expected Go
document and tests the public parser against the exact digest. Do not derive the
expected vector from the parser or maintain another JSON copy.
