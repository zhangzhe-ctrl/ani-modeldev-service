package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := Run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func strictJSON(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return fmt.Errorf("invalid bounded maintenance JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing maintenance JSON")
	}
	return nil
}
func ownerPool(ctx context.Context, file string, deleteRole string, modeldev bool) (*pgxpool.Pool, error) {
	raw, err := readBounded(file, 65536, true)
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("invalid private maintenance database reference")
	}
	if err = configureDeleteOwner(config, deleteRole, modeldev); err != nil {
		return nil, err
	}
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("maintenance owner database unavailable")
	}
	return pool, nil
}

// Run is the one-time CLI seam. Original inventories and caller-reviewed hashes
// are mandatory; there is no tenant-wide, owner-wide, empty-scope or SQL option.
func Run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("legacy-modeldev-data-cleanup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mdFile := flags.String("modeldev-dsn-file", "", "private existing MD maintenance owner reference (0600)")
	govFile := flags.String("governance-dsn-file", "", "private existing Governance maintenance owner reference (0600)")
	original := flags.String("original-inventory-file", "", "fixed task09 old-inventory.json")
	originalGov := flags.String("original-governance-inventory-file", "", "fixed task09 old-gov-inventory.json")
	syncFile := flags.String("original-governance-sync-file", "", "fixed original23 ended/ACKed usage-sync manifest")
	syncSHA := flags.String("original-governance-sync-sha256", "", "root-reviewed original23 manifest byte SHA256")
	f216Mode := flags.String("original-f216", "", "finite original f216 plan/apply only")
	f216Config := flags.String("f216-owner-config-file", "", "original private ModelDev mTLS owner files")
	f216Journal := flags.String("f216-journal-file", "", "new durable node-local journal path")
	planByteSHA := flags.String("manifest-byte-sha256", "", "root-reviewed f216 plan file bytes")
	mdDeleteRole := flags.String("modeldev-effective-delete-owner", "", "explicit existing modeldev_v21_migrate; original postgres SET ROLE only")
	govDeleteRole := flags.String("governance-effective-delete-owner", "", "explicit existing gov_migrate; original postgres SET ROLE only")
	phase := flags.String("phase", "governance", "governance first, then modeldev")
	rootFile := flags.String("root-evidence-file", "", "root-verified external cleanup/stopped-source receipt")
	rootSHA := flags.String("root-evidence-sha256", "", "root's independently reviewed actual receipt byte SHA256")
	priorFile := flags.String("governance-receipt-file", "", "successful first-phase original Governance deletion receipt")
	priorSHA := flags.String("governance-receipt-sha256", "", "root-reviewed first-phase receipt byte SHA256")
	apply := flags.Bool("apply", false, "delete only exact qualified reviewed manifest rows")
	manifestFile := flags.String("manifest-file", "", "saved read-only stage plan JSON, mandatory apply")
	manifestSHA := flags.String("manifest-sha256", "", "ManifestSHA256 from the saved read-only stage plan")
	if flags.Parse(args) != nil || len(flags.Args()) != 0 || *mdFile == "" || *govFile == "" || *original == "" || *originalGov == "" {
		return fmt.Errorf("explicit two private owner DSN files and fixed original inventory files required")
	}
	scope, err := loadOriginal(*original, *originalGov)
	if err != nil {
		return err
	}
	if *syncFile != "" || *syncSHA != "" {
		if err = loadOriginalSync(*syncFile, *syncSHA, &scope); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	md, err := ownerPool(ctx, *mdFile, *mdDeleteRole, true)
	if err != nil {
		return err
	}
	defer md.Close()
	gov, err := ownerPool(ctx, *govFile, *govDeleteRole, false)
	if err != nil {
		return err
	}
	defer gov.Close()
	command := &CleanupCommand{ModelDev: md, Governance: gov, Original: scope}
	if *f216Mode != "" {
		if *apply {
			return fmt.Errorf("f216 and DB deletion actions cannot be combined")
		}
		return command.runF216(ctx, *f216Mode, *f216Config, *manifestFile, *planByteSHA, *rootFile, *rootSHA, *f216Journal, output)
	}
	var prior *DeletionReceipt
	if *rootFile != "" {
		raw, err := readBounded(*rootFile, 1<<20, false)
		if err != nil {
			return err
		}
		if *rootSHA == "" || bytesSHA(raw) != *rootSHA {
			return fmt.Errorf("root independently approved external receipt byte hash required")
		}
		var root RootEvidence
		if err = strictJSON(raw, &root); err != nil {
			return err
		}
		command.Root = &root
		command.RootSHA256 = *rootSHA
	}
	if *priorFile != "" {
		raw, err := readBounded(*priorFile, 4<<20, false)
		if err != nil {
			return err
		}
		if *priorSHA == "" || bytesSHA(raw) != *priorSHA {
			return fmt.Errorf("root independently approved Governance receipt byte hash required")
		}
		var receipt DeletionReceipt
		if err = strictJSON(raw, &receipt); err != nil {
			return err
		}
		prior = &receipt
		command.GovernanceReceiptSHA256 = *priorSHA
	}
	encoder := json.NewEncoder(output)
	if !*apply {
		plan, err := command.Plan(ctx, *phase, prior)
		if err != nil {
			return err
		}
		return encoder.Encode(plan)
	}
	raw, err := readBounded(*manifestFile, 4<<20, false)
	if err != nil {
		return fmt.Errorf("saved complete stage manifest required")
	}
	var reviewed CleanupPlan
	if err = strictJSON(raw, &reviewed); err != nil {
		return err
	}
	if *manifestSHA == "" || reviewed.ManifestSHA256 != *manifestSHA || reviewed.Phase != *phase {
		return fmt.Errorf("explicit reviewed stage ManifestSHA256 differs")
	}
	receipt, err := command.Apply(ctx, reviewed, prior)
	if err != nil {
		if *phase == "modeldev" && validatePriorReceipt(prior, scope) == nil {
			return fmt.Errorf("PARTIAL_GOVERNANCE_COMMITTED; MODELDEV_NOT_CONFIRMED: %w", err)
		}
		return err
	}
	return encoder.Encode(receipt)
}
