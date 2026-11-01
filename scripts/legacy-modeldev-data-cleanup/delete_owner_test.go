package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOriginalDeleteOwnerIsSeparateFromRuntimeReadQualification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original-postgres-owner-proof.json")
	raw := []byte("root-reviewed exact original PostgreSQL source UID and authenticated/effective owner observation")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("owner proof unavailable")
	}
	source := SourceProof{Kind: "ORIGINAL_POSTGRES_DELETE_OWNER", Path: path, SHA256: bytesSHA(raw)}
	for _, component := range []string{"modeldev", "governance"} {
		runtime, owner, db, oid := "modeldev_v21_runtime", "modeldev_v21_migrate", "ani_modeldev_v21", uint32(56512)
		if component == "governance" {
			runtime, owner, db, oid = "gov_runtime", "gov_migrate", "ani_governance", 16484
		}
		read := DatabaseIdentity{Database: db, Schema: "public", Role: runtime, ServerAddress: "10.16.0.63", Port: 5432, DatabaseOID: oid}
		current := read
		current.Role = owner
		proof := DeleteOwnerProof{AuthenticatedRole: "postgres", EffectiveRole: owner, PostgresStatefulSetUID: "3ab646a6-9c8d-488c-9f8b-9ce659aa4d37", PostgresPodUID: "333fe87e-4a47-4e99-b5ea-33ef48be2e27", PostgresServiceUID: "a211aa89-9521-4570-836f-b9dbb1755e79", AdminSecretUID: "c0f61772-2154-46cd-88fc-4018ce43a9f2", Source: source}
		c := &CleanupCommand{Root: &RootEvidence{DeleteOwners: map[string]DeleteOwnerProof{component: proof}}}
		if !deleteOwnerValid(proof, DatabaseSnapshot{Identity: current, AuthenticatedRole: "postgres"}, owner) || !c.readQualificationIdentityMatches(read, current, component) {
			t.Fatal("same original location with explicit existing owner refused", component)
		}
		if read.Role != runtime {
			t.Fatal("runtime journal identity was rewritten")
		}
		for _, kind := range []string{"wrong_role", "wrong_database", "wrong_oid", "wrong_schema", "wrong_server", "missing_proof", "wrong_auth", "wrong_source_uid"} {
			badCurrent := current
			badProof := proof
			switch kind {
			case "wrong_role":
				badCurrent.Role = "unapproved_owner"
			case "wrong_database":
				badCurrent.Database = "other"
			case "wrong_oid":
				badCurrent.DatabaseOID++
			case "wrong_schema":
				badCurrent.Schema = "other"
			case "wrong_server":
				badCurrent.ServerAddress = "10.16.0.64"
			case "missing_proof":
				badProof.Source.SHA256 = ""
			case "wrong_auth":
				badProof.AuthenticatedRole = "arbitrary_user"
			case "wrong_source_uid":
				badProof.PostgresPodUID = "recreated-pod"
			}
			c.Root.DeleteOwners[component] = badProof
			if c.readQualificationIdentityMatches(read, badCurrent, component) {
				t.Fatal("foreign read/deletion identity accepted", component, kind)
			}
		}
		if deleteOwnerValid(proof, DatabaseSnapshot{Identity: current, AuthenticatedRole: "arbitrary_user"}, owner) {
			t.Fatal("actual authenticated role was ignored")
		}
		direct := proof
		direct.AuthenticatedRole = owner
		if !deleteOwnerValid(direct, DatabaseSnapshot{Identity: current, AuthenticatedRole: owner}, owner) {
			t.Fatal("existing direct owner refused")
		}
	}
}
func TestDeleteOwnerSelectionDoesNotAddPrincipalOrAllowArbitraryRole(t *testing.T) {
	for _, md := range []bool{true, false} {
		expected := "gov_migrate"
		if md {
			expected = "modeldev_v21_migrate"
		}
		for _, role := range []string{"", expected, "postgres", "new_role", "gov_runtime"} {
			config := &pgxpool.Config{}
			err := configureDeleteOwner(config, role, md)
			if role == "" {
				if err != nil || config.AfterConnect != nil {
					t.Fatal("ordinary runtime connection was changed")
				}
			} else if role == expected {
				if err != nil || config.AfterConnect == nil {
					t.Fatal("explicit original effective owner missing")
				}
			} else if err == nil {
				t.Fatal("unapproved SET ROLE target accepted", role)
			}
		}
	}
}
