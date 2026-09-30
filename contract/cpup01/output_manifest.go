package cpup01

// OutputFile describes independently collected bytes. It contains neither an
// object-store ETag nor a signed URL. This inventory alone proves no upload.
type OutputFile struct {
	RelativePath string `json:"relative_path"`
	Role string `json:"role"`
	SizeBytes int64 `json:"size_bytes,string"`
	SHA256 string `json:"sha256"`
}

// OutputManifestBytes binds a complete collector inventory to the immutable
// admission. The returned manifest has no self hash and cannot claim PUBLISHED.
func OutputManifestBytes(admission AdmissionEnvelope, files []OutputFile) ([]byte, string, error) {
	return nil, "", ErrInvalidArgument
}
