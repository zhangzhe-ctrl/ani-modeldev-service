package workspace

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
	"golang.org/x/sys/unix"
)

// PrepareInput is called by the registered prepare component on its own mounted
// execution PVC. The directory is component configuration, never an RPC field.
func PrepareInput(ctx context.Context, client *s3.Client, execution biz.Execution, binding biz.WorkspaceBinding, directory string) (biz.WorkspaceBinding, error) {
	if ctx == nil || client == nil {
		return biz.WorkspaceBinding{}, biz.ErrInputSourceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return biz.WorkspaceBinding{}, err
	}
	if _, _, err := execution.CanonicalPayloads(); err != nil {
		return biz.WorkspaceBinding{}, biz.ErrInvalidAdmission
	}
	want := execution.Snapshot.Workspace
	if binding.Mode != want.Mode || binding.NamespaceName != execution.Snapshot.Environment.NamespaceName || binding.NamespaceUID != execution.Snapshot.Environment.NamespaceUID ||
		binding.PVCName == "" || binding.PVCUID == "" || binding.InputSubpath != want.InputSubpath || binding.TrainingSubpath != want.TrainingSubpath ||
		binding.ReportsSubpath != want.ReportsSubpath || binding.PublicationSubpath != want.PublicationSubpath {
		return biz.WorkspaceBinding{}, biz.ErrRuntimeConflict
	}
	root, err := openPipelineRoot(directory)
	if err != nil {
		return biz.WorkspaceBinding{}, err
	}
	defer unix.Close(root)
	input := execution.Snapshot.Input
	// The immutable execution fixes this exact key and version; this narrowed
	// scope grants no authority to read arbitrary component-selected objects.
	scope := cpup01.StorageScope{StorageConnectionID: input.Object.StorageConnectionID, Bucket: input.Object.Bucket, ApprovedPrefix: path.Dir(input.Object.Key)}
	verified, err := objectstore.NewVerifier(client, input.Object.StorageConnectionID, 32*1024*1024).VerifyCSV(ctx, scope, input.Object)
	if err != nil {
		return biz.WorkspaceBinding{}, err
	}
	if verified.SchemaVersion != input.SchemaVersion || verified.RowCount != input.RowCount || verified.FeatureCount != input.FeatureCount {
		return biz.WorkspaceBinding{}, biz.ErrInputVerification
	}
	// The second read remains fixed to the verified version and is independently
	// measured before it can become the trainer's local input.
	response, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(input.Object.Bucket), Key: aws.String(input.Object.Key), VersionId: input.Object.VersionID})
	if err != nil {
		return biz.WorkspaceBinding{}, pipelineIOError(ctx, biz.ErrInputSourceUnavailable)
	}
	if response == nil || response.Body == nil {
		return biz.WorkspaceBinding{}, biz.ErrInputSourceUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: response.Body}, input.Object.SizeBytes+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return biz.WorkspaceBinding{}, pipelineIOError(ctx, biz.ErrInputSourceUnavailable)
	}
	if response.VersionId == nil || input.Object.VersionID == nil || *response.VersionId != *input.Object.VersionID || int64(len(data)) != input.Object.SizeBytes || pipelineContentHash(data) != input.Object.SHA256 {
		return biz.WorkspaceBinding{}, biz.ErrInputVerification
	}
	manifest, err := PreparedManifestBytes(execution, binding)
	if err != nil {
		return biz.WorkspaceBinding{}, biz.ErrInputVerification
	}
	inputs, err := openPipelineSubdirectory(root, want.InputSubpath)
	if err != nil {
		return biz.WorkspaceBinding{}, err
	}
	defer unix.Close(inputs)
	reports, err := openPipelineSubdirectory(root, want.ReportsSubpath)
	if err != nil {
		return biz.WorkspaceBinding{}, err
	}
	defer unix.Close(reports)
	if err := writePipelineFile(ctx, inputs, "data.csv", data); err != nil {
		return biz.WorkspaceBinding{}, err
	}
	if err := writePipelineFile(ctx, reports, "prepared-manifest.json", manifest); err != nil {
		return biz.WorkspaceBinding{}, err
	}
	binding.PreparedManifestSHA256, binding.PreparedManifestBytes = pipelineContentHash(manifest), int64(len(manifest))
	return binding, nil
}

// PreparedManifestBytes is the shared byte contract for the prepare component
// and the owner's independent verification. Encoding alone proves no I/O or
// permission to use the claimed workspace.
func PreparedManifestBytes(execution biz.Execution, binding biz.WorkspaceBinding) ([]byte, error) {
	input := execution.Snapshot.Input
	return json.Marshal(struct {
		Schema         string `json:"schema"`
		TenantID       string `json:"tenant_id"`
		OperationID    string `json:"operation_id"`
		ExecutionID    string `json:"execution_id"`
		SpecHash       string `json:"execution_spec_hash"`
		InputVersionID string `json:"input_version_id"`
		InputSHA256    string `json:"input_sha256"`
		InputBytes     int64  `json:"input_size_bytes"`
		NamespaceUID   string `json:"namespace_uid"`
		PVCUID         string `json:"pvc_uid"`
	}{"ani.modeldev.prepared-manifest.v1", execution.TenantID, execution.OperationID, execution.ExecutionID, execution.SpecHash,
		input.InputVersionID, input.Object.SHA256, input.Object.SizeBytes, binding.NamespaceUID, binding.PVCUID})
}

// UploadOutput sends independently collected actual files and the manifest.
// This returns a candidate; only a later step may ask the owner to verify and
// publish after the uploader container has actually terminated successfully.
func UploadOutput(ctx context.Context, client *s3.Client, execution biz.Execution, directory string, output biz.CollectedOutput) (biz.RuntimePublication, error) {
	if ctx == nil || client == nil {
		return biz.RuntimePublication{}, biz.ErrObjectSourceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return biz.RuntimePublication{}, err
	}
	manifest, manifestSHA, err := cpup01.OutputManifestBytes(cpup01.AdmissionEnvelope(execution.Admission), output.Files)
	if err != nil || output.ExecutionID != execution.ExecutionID || output.ExecutionSpecHash != execution.SpecHash || output.StorageState != "WORKSPACE_ONLY" ||
		!bytes.Equal(output.Manifest, manifest) || output.ManifestSHA256 != manifestSHA {
		return biz.RuntimePublication{}, biz.ErrInvalidWorkspaceOutput
	}
	scope := execution.Snapshot.PublicationScope
	if !biz.ValidStorageKey(scope.ApprovedPrefix) {
		return biz.RuntimePublication{}, biz.ErrInvalidWorkspaceOutput
	}
	root, err := openPipelineRoot(directory)
	if err != nil {
		return biz.RuntimePublication{}, err
	}
	defer unix.Close(root)
	files := append([]cpup01.OutputFile{}, output.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	// Capture and validate all inputs before emitting any upload. Unlinked
	// temporary files bound memory use and keep later source mutation out of PUT.
	captured := make([]*os.File, 0, len(files))
	defer func() {
		for _, file := range captured {
			_ = file.Close()
		}
	}()
	for _, file := range files {
		capture, err := capturePipelineFile(ctx, root, file)
		if err != nil {
			return biz.RuntimePublication{}, err
		}
		captured = append(captured, capture)
	}
	var bundle *os.File
	var bundleBytes int64
	var bundleHash string
	if execution.Snapshot.OutputContract.CreateTarBundle {
		bundle, bundleBytes, bundleHash, err = bundlePipelineFiles(ctx, files, captured, manifest)
		if err != nil {
			return biz.RuntimePublication{}, err
		}
		defer bundle.Close()
	}
	candidate := biz.RuntimePublication{LogicalKey: "cpu-p01:" + execution.ExecutionID}
	prefix := scope.ApprovedPrefix + "/" + execution.ExecutionID + "/"
	for i, file := range files {
		object, err := uploadPipelineObject(ctx, client, scope, prefix+file.RelativePath, file.SizeBytes, file.SHA256, captured[i])
		if err != nil {
			return biz.RuntimePublication{}, err
		}
		candidate.Files = append(candidate.Files, biz.PublishedRuntimeFile{File: file, Object: object})
	}
	candidate.Manifest, err = uploadPipelineObject(ctx, client, scope, prefix+"output-manifest.json", int64(len(manifest)), manifestSHA, bytes.NewReader(manifest))
	if err != nil {
		return biz.RuntimePublication{}, err
	}
	if bundle != nil {
		object, err := uploadPipelineObject(ctx, client, scope, prefix+"bundle.tar", bundleBytes, bundleHash, bundle)
		if err != nil {
			return biz.RuntimePublication{}, err
		}
		candidate.Bundle = &object
	}
	return candidate, nil
}

func openPipelineRoot(directory string) (int, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || directory == "/" {
		return -1, biz.ErrInvalidWorkspaceOutput
	}
	root, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, biz.ErrInvalidWorkspaceOutput
	}
	for _, component := range strings.Split(strings.TrimPrefix(directory, "/"), "/") {
		next, err := unix.Openat(root, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(root)
		if err != nil {
			return -1, biz.ErrInvalidWorkspaceOutput
		}
		root = next
	}
	return root, nil
}

func openPipelineSubdirectory(root int, subpath string) (int, error) {
	if !biz.ValidStorageKey(subpath) {
		return -1, biz.ErrInvalidWorkspaceOutput
	}
	current, err := unix.Dup(root)
	if err != nil {
		return -1, biz.ErrInvalidWorkspaceOutput
	}
	unix.CloseOnExec(current)
	for _, component := range strings.Split(subpath, "/") {
		if err := unix.Mkdirat(current, component, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
			_ = unix.Close(current)
			return -1, biz.ErrInvalidWorkspaceOutput
		}
		next, err := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(current)
		if err != nil {
			return -1, biz.ErrInvalidWorkspaceOutput
		}
		current = next
	}
	return current, nil
}

func writePipelineFile(ctx context.Context, root int, name string, content []byte) error {
	temporary := ".cpu-p01-" + uuid.NewString()
	fd, err := unix.Openat(root, temporary, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return biz.ErrInvalidWorkspaceOutput
	}
	file := os.NewFile(uintptr(fd), temporary)
	defer file.Close()
	defer unix.Unlinkat(root, temporary, 0)
	if _, err := io.Copy(file, contextReader{ctx: ctx, reader: bytes.NewReader(content)}); err != nil {
		return pipelineIOError(ctx, biz.ErrInvalidWorkspaceOutput)
	}
	if err := file.Sync(); err != nil {
		return biz.ErrInvalidWorkspaceOutput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Linkat atomically creates a previously absent name on the same mounted
	// filesystem, including CephFS where RENAME_NOREPLACE is not supported.
	// It cannot replace an existing target. Remove our temporary name before
	// verification so the published file must have exactly one remaining link.
	if err := unix.Linkat(root, temporary, root, name, 0); err != nil && !errors.Is(err, unix.EEXIST) {
		return biz.ErrInvalidWorkspaceOutput
	}
	if err := unix.Unlinkat(root, temporary, 0); err != nil {
		return biz.ErrInvalidWorkspaceOutput
	}
	// Both new publication and retries require the actual bytes and inode
	// constraints. A crash leaving two names remains untrusted; never remove
	// an arbitrary alias or overwrite a conflicting file to make a retry pass.
	existing, err := capturePipelineFile(ctx, root, cpup01.OutputFile{RelativePath: name, SizeBytes: int64(len(content)), SHA256: pipelineContentHash(content)})
	if err != nil {
		return err
	}
	_ = existing.Close()
	if err := unix.Fsync(root); err != nil {
		return biz.ErrInvalidWorkspaceOutput
	}
	return nil
}

func capturePipelineFile(ctx context.Context, root int, expected cpup01.OutputFile) (*os.File, error) {
	if expected.SizeBytes <= 0 || expected.RelativePath == "" || strings.ContainsAny(expected.RelativePath, "/\\") || expected.RelativePath == "." || expected.RelativePath == ".." {
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	fd, err := unix.Openat(root, expected.RelativePath, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	source := os.NewFile(uintptr(fd), expected.RelativePath)
	defer source.Close()
	var before unix.Stat_t
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Size != expected.SizeBytes {
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	staged, err := newPipelineTemporary()
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = staged.Close()
		}
	}()
	digest := sha256.New()
	reader := contextReader{ctx: ctx, reader: source}
	size, err := io.Copy(io.MultiWriter(staged, digest), io.LimitReader(reader, expected.SizeBytes))
	if err != nil {
		return nil, pipelineIOError(ctx, biz.ErrInvalidWorkspaceOutput)
	}
	var extra [1]byte
	n, readErr := reader.Read(extra[:])
	var after unix.Stat_t
	if n != 0 || readErr != io.EOF || size != expected.SizeBytes || hex.EncodeToString(digest.Sum(nil)) != expected.SHA256 ||
		unix.Fstat(fd, &after) != nil || after.Mode&unix.S_IFMT != unix.S_IFREG || after.Size != before.Size || after.Nlink != 1 || after.Mtim != before.Mtim || after.Ctim != before.Ctim {
		return nil, pipelineIOError(ctx, biz.ErrInvalidWorkspaceOutput)
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	ok = true
	return staged, nil
}

func newPipelineTemporary() (*os.File, error) {
	file, err := os.CreateTemp("", "cpu-p01-bytes-*")
	if err != nil {
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	// Linux keeps the descriptor alive after unlink. The component's configured
	// TMPDIR owns the temporary storage; no mutable pathname remains to replace.
	if err := os.Remove(file.Name()); err != nil {
		_ = file.Close()
		return nil, biz.ErrInvalidWorkspaceOutput
	}
	return file, nil
}

func bundlePipelineFiles(ctx context.Context, files []cpup01.OutputFile, captured []*os.File, manifest []byte) (*os.File, int64, string, error) {
	bundle, err := newPipelineTemporary()
	if err != nil {
		return nil, 0, "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = bundle.Close()
		}
	}()
	writer := tar.NewWriter(bundle)
	for i, file := range files {
		if err := writer.WriteHeader(&tar.Header{Name: file.RelativePath, Mode: 0600, Size: file.SizeBytes, Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
			return nil, 0, "", biz.ErrInvalidWorkspaceOutput
		}
		if _, err := captured[i].Seek(0, io.SeekStart); err != nil {
			return nil, 0, "", biz.ErrInvalidWorkspaceOutput
		}
		if _, err := io.Copy(writer, contextReader{ctx: ctx, reader: captured[i]}); err != nil {
			return nil, 0, "", pipelineIOError(ctx, biz.ErrInvalidWorkspaceOutput)
		}
	}
	// The manifest hashes its file members. The bundle has a separate hash and
	// never appears in its own manifest or tar, avoiding recursive identities.
	if err := writer.WriteHeader(&tar.Header{Name: "output-manifest.json", Mode: 0600, Size: int64(len(manifest)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}); err != nil {
		return nil, 0, "", biz.ErrInvalidWorkspaceOutput
	}
	if _, err := writer.Write(manifest); err != nil {
		return nil, 0, "", biz.ErrInvalidWorkspaceOutput
	}
	if err := writer.Close(); err != nil {
		return nil, 0, "", biz.ErrInvalidWorkspaceOutput
	}
	if _, err := bundle.Seek(0, io.SeekStart); err != nil {
		return nil, 0, "", biz.ErrInvalidWorkspaceOutput
	}
	digest := sha256.New()
	size, err := io.Copy(digest, contextReader{ctx: ctx, reader: bundle})
	if err != nil {
		return nil, 0, "", pipelineIOError(ctx, biz.ErrInvalidWorkspaceOutput)
	}
	if _, err := bundle.Seek(0, io.SeekStart); err != nil {
		return nil, 0, "", biz.ErrInvalidWorkspaceOutput
	}
	ok = true
	return bundle, size, hex.EncodeToString(digest.Sum(nil)), nil
}

func uploadPipelineObject(ctx context.Context, client *s3.Client, scope cpup01.StorageScope, key string, size int64, digest string, content io.ReadSeeker) (cpup01.FixedObjectRef, error) {
	if !biz.ValidStorageKey(key) || !strings.HasPrefix(key, scope.ApprovedPrefix+"/") {
		return cpup01.FixedObjectRef{}, biz.ErrInvalidWorkspaceOutput
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return cpup01.FixedObjectRef{}, biz.ErrInvalidWorkspaceOutput
	}
	object := cpup01.FixedObjectRef{StorageConnectionID: scope.StorageConnectionID, Bucket: scope.Bucket, Key: key, SizeBytes: size, SHA256: digest}
	response, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(scope.Bucket), Key: aws.String(key), Body: content, ContentLength: aws.Int64(size), IfNoneMatch: aws.String("*")})
	if err == nil {
		if response == nil || response.VersionId == nil || !validPipelineVersion(*response.VersionId) {
			return cpup01.FixedObjectRef{}, biz.ErrObjectSourceUnavailable
		}
		version := *response.VersionId
		object.VersionID = &version
		return object, nil
	}
	var apiError smithy.APIError
	if !errors.As(err, &apiError) || apiError.ErrorCode() != "PreconditionFailed" {
		return cpup01.FixedObjectRef{}, pipelineIOError(ctx, biz.ErrObjectSourceUnavailable)
	}
	// A retry may reuse only an existing immutable version with the same actual
	// byte identity. HEAD supplies the version, never the content digest.
	existing, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(scope.Bucket), Key: aws.String(key)})
	if err != nil {
		return cpup01.FixedObjectRef{}, pipelineIOError(ctx, biz.ErrObjectSourceUnavailable)
	}
	if existing == nil || existing.VersionId == nil || !validPipelineVersion(*existing.VersionId) {
		return cpup01.FixedObjectRef{}, biz.ErrObjectVerification
	}
	version := *existing.VersionId
	object.VersionID = &version
	verified, err := objectstore.NewVerifier(client, scope.StorageConnectionID, size).Verify(ctx, scope, object)
	if err != nil {
		return cpup01.FixedObjectRef{}, err
	}
	return verified.Object, nil
}

func validPipelineVersion(version string) bool {
	return biz.ValidStorageReference(version) && !strings.EqualFold(version, "null")
}
func pipelineContentHash(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
func pipelineIOError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fallback
}
