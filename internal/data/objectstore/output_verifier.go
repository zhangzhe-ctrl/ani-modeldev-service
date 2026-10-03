package objectstore

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// VerifyOutputBundle reads the immutable archive without extracting paths.
// Each regular entry must match the already frozen inventory or canonical
// manifest. Outer object SHA/size/version remain independently verified.
func (v *Verifier) VerifyOutputBundle(ctx context.Context,scope cpup01.StorageScope,object cpup01.FixedObjectRef,files []cpup01.OutputFile,manifest []byte) (biz.VerifiedObject,error) {
	wanted := make(map[string]cpup01.OutputFile,len(files)+1)
	for _,file := range files {
		if file.RelativePath == "output-manifest.json" || wanted[file.RelativePath].RelativePath != "" || !biz.ValidStorageKey(file.RelativePath) || file.SizeBytes <= 0 { return biz.VerifiedObject{},biz.ErrObjectVerification }
		wanted[file.RelativePath] = file
	}
	manifestDigest := sha256.Sum256(manifest)
	wanted["output-manifest.json"] = cpup01.OutputFile{RelativePath:"output-manifest.json",SizeBytes:int64(len(manifest)),SHA256:hex.EncodeToString(manifestDigest[:])}
	return v.verify(ctx,scope,object,func(stream io.Reader) error {
		reader := tar.NewReader(stream)
		seen := make(map[string]bool,len(wanted))
		for {
			header,err := reader.Next()
			if err == io.EOF { break }
			if err != nil { return biz.ErrObjectVerification }
			file,ok := wanted[header.Name]
			if !ok || seen[header.Name] || header.Typeflag != tar.TypeReg || header.Linkname != "" || header.Size != file.SizeBytes || header.Format != tar.FormatUSTAR || len(header.PAXRecords) != 0 { return biz.ErrObjectVerification }
			seen[header.Name] = true
			digest := sha256.New()
			count,err := io.Copy(digest,reader)
			if err != nil || count != file.SizeBytes || hex.EncodeToString(digest.Sum(nil)) != file.SHA256 { return biz.ErrObjectVerification }
		}
		if len(seen) != len(wanted) { return biz.ErrObjectVerification }
		// tar permits zero padding after its end marker; nonzero trailing bytes
		// would conceal a second archive or unrelated payload.
		var padding [4096]byte
		for {
			count,err := stream.Read(padding[:])
			for _,value := range padding[:count] { if value != 0 { return biz.ErrObjectVerification } }
			if err == io.EOF { return nil }
			if err != nil || count == 0 { return biz.ErrObjectVerification }
		}
	})
}
