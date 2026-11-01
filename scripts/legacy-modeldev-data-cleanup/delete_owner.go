package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeleteOwnerProof struct {
	AuthenticatedRole, EffectiveRole                                           string
	PostgresStatefulSetUID, PostgresPodUID, PostgresServiceUID, AdminSecretUID string
	Source                                                                     SourceProof
}

func sameDatabaseLocation(a, b DatabaseIdentity) bool {
	return a.Database == b.Database && a.Schema == b.Schema && a.ServerAddress == b.ServerAddress && a.Port == b.Port && a.DatabaseOID == b.DatabaseOID
}
func deleteOwnerValid(proof DeleteOwnerProof, snapshot DatabaseSnapshot, owner string) bool {
	return proof.EffectiveRole == owner && snapshot.Identity.Role == owner && proof.AuthenticatedRole == snapshot.AuthenticatedRole && (proof.AuthenticatedRole == owner || proof.AuthenticatedRole == "postgres") &&
		proof.PostgresStatefulSetUID == "3ab646a6-9c8d-488c-9f8b-9ce659aa4d37" && proof.PostgresPodUID == "333fe87e-4a47-4e99-b5ea-33ef48be2e27" && proof.PostgresServiceUID == "a211aa89-9521-4570-836f-b9dbb1755e79" && proof.AdminSecretUID == "c0f61772-2154-46cd-88fc-4018ce43a9f2" && proof.Source.Kind == "ORIGINAL_POSTGRES_DELETE_OWNER" && proofValid(proof.Source)
}
func (c *CleanupCommand) readQualificationIdentityMatches(read, current DatabaseIdentity, component string) bool {
	if read == current {
		return true
	}
	if !sameDatabaseLocation(read, current) || c.Root == nil {
		return false
	}
	runtime, owner := "modeldev_v21_runtime", "modeldev_v21_migrate"
	if component == "governance" {
		runtime, owner = "gov_runtime", "gov_migrate"
	}
	proof, ok := c.Root.DeleteOwners[component]
	// The authenticated role is explicitly bound by the independent root owner
	// proof; actual snapshot authentication is checked by rootBlockers as well.
	return ok && read.Role == runtime && deleteOwnerValid(proof, DatabaseSnapshot{Identity: current, AuthenticatedRole: proof.AuthenticatedRole}, owner)
}
func configureDeleteOwner(pool *pgxpool.Config, role string, modeldev bool) error {
	if role == "" {
		return nil
	}
	expected := "gov_migrate"
	if modeldev {
		expected = "modeldev_v21_migrate"
	}
	if role != expected {
		return fmt.Errorf("only the existing exact database migration owner can be selected")
	}
	pool.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		var authenticated, effective string
		if err := connection.QueryRow(ctx, "SELECT session_user,current_user").Scan(&authenticated, &effective); err != nil {
			return err
		}
		if authenticated != expected && authenticated != "postgres" || effective != authenticated {
			return fmt.Errorf("unapproved authenticated maintenance database role")
		}
		if authenticated == "postgres" {
			if _, err := connection.Exec(ctx, "SET ROLE "+pgx.Identifier{expected}.Sanitize()); err != nil {
				return fmt.Errorf("original maintenance owner role unavailable")
			}
		}
		if err := connection.QueryRow(ctx, "SELECT current_user").Scan(&effective); err != nil || effective != expected {
			return fmt.Errorf("effective maintenance owner role differs")
		}
		return nil
	}
	return nil
}
