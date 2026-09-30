package biz

// Source-unavailable errors describe an inability to obtain a complete source
// observation. They retain the existing verification error contract without
// retaining storage response text, endpoint URLs or credential-bearing errors.
var (
	ErrObjectSourceUnavailable error = &verificationSourceError{"OBJECT_SOURCE_UNAVAILABLE", ErrObjectVerification}
	ErrInputSourceUnavailable  error = &verificationSourceError{"INPUT_SOURCE_UNAVAILABLE", ErrInputVerification}
)

type verificationSourceError struct {
	code   string
	parent error
}

func (err *verificationSourceError) Error() string { return err.code }
func (err *verificationSourceError) Unwrap() error { return err.parent }
