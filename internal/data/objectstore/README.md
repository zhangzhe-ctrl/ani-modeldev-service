# CPU-P01 object byte verification

This adapter uses the official AWS SDK for Go v2 S3 client, pinned to service/s3
v1.113.4 with checksums resolved on Fedora. Reference API:
https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3@v1.113.4#Client.GetObject.
The composition root supplies an authenticated client, a known connection ID,
and a positive maximum object size derived from owner-controlled configuration.
Component-provided references cannot select endpoints or credentials.

The owner supplies the approved bucket and execution prefix. The caller must
derive that execution prefix from the frozen scope and current execution; it is
not an authorization field accepted from an ordinary user. The adapter checks
connection, bucket, canonical relative key, and the complete prefix boundary
before making a request. Every read requires an explicit non-null VersionID and
an expected nonzero size and lowercase SHA256. The initial implementation does
not accept an immutable-copy assertion as a substitute for storage immutability.

Verification issues an actual GetObject for that version and checks the returned
version, streams the expected number of bytes into SHA256, and checks EOF. Extra,
missing, corrupt, or differently versioned bytes fail; ETag is not used as a
content digest. The configured byte ceiling is checked before network access.
Cancellation reaches the remote body reader and never produces a receipt.

VerifiedObject is an observation, not PUBLISHED. The owner must separately verify
uploader completion, match this reference to the collected file/manifest and
execution, persist the primary publication transaction, and enforce current
download authorization. Those uses are not yet wired to product transports.
The adapter reads only and never cleans up local output or remote artifacts.

Tests exercise the official SDK over a TLS httptest server using synthetic bytes
and test-only anonymous credentials. The server is an explicit external-system
test double. This proves request, correlation, streaming, and rejection behavior;
it does not prove a deployed S3 service, real tenant IAM, upload completion,
publication persistence, or a BFF-authorized independent verifier Job.

`VerifyCSV` consumes the same fixed-version stream and verifies the registered
`ani.cpu.csv.v1` format: the exact x0 through x15, label header; 1024 samples;
16 finite decimal features representable as float32; and labels 0 or 1. The
declared input must fit both the owner's read budget and the 32 MiB CSV ceiling.
Blank physical lines, multiline numeric cells, hexadecimal floats, invalid
records, missing or extra samples, and non-finite values are rejected. The
inspection must finish before actual length, SHA256 and EOF can produce a
`VerifiedCSV` observation. It neither creates an input version nor marks READY.

Fedora module evidence at source `3910d3312093c62b4c004865abeea07e1d4bf40f`
passes all object/CSV tests with clean formatting. Initial CSV behavior was RED
at `94601750d6b00dca0c102a92869a2b4ce8781ccb`; additional blank-line and
hexadecimal-float rejection was RED at
`bf13d80251e070a712ea8c480d69b4edf3e15409`. These are synthetic SDK-boundary
tests, not a real S3 import or persistent READY proof.
