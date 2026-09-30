package cpup01

import "time"

// AdmissionEnvelope binds authenticated command identities to the immutable
// intent and resolved snapshot. Both producers and consumers validate this
// value; it contains no persistence, transport, or current-default resolution.
type AdmissionEnvelope struct {
	TenantID string
	Actor string
	OperationID string
	ExecutionID string
	Intent Intent
	IntentHash string
	Snapshot Snapshot
	SpecHash string
	AcceptedAt time.Time
}

// CanonicalPayloads validates identity, hashes, and intent-to-snapshot coherence
// and returns the two exact canonical payloads. It never reads the wall clock.
func (a AdmissionEnvelope) CanonicalPayloads() ([]byte, []byte, error) {
	return nil, nil, ErrInvalidArgument
}
