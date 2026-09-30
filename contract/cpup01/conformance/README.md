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
