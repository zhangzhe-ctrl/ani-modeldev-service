package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

var mdOrder = []string{"modeldev_execution_cleanup_audits", "modeldev_close_intents", "modeldev_execution_runtimes", "modeldev_run_authorities", "modeldev_pipeline_confirmed_runs", "modeldev_pipeline_dispatches", "modeldev_executions", "modeldev_execution_identities", "modeldev_input_versions"}
var govOrder = []string{"sys_modeldev_acceptances", "sys_quota_charges", "sys_gpu_usage_sync", "sys_quota_operations", "sys_quota_release_receipts"}

type tableRows struct {
	table string
	rows  []row
	raw   []json.RawMessage
}
type databaseView struct {
	Snapshot DatabaseSnapshot
	Tables   []tableRows
	Accounts []row
	Catalog  any
}
type actualCatalog struct {
	Columns       []column
	ForeignKeys   []foreignKey
	PrimaryKeys   map[string][]string
	Triggers      []trigger
	TriggerBodies map[string]string
	Constraints   []string
	Rules         []string
	Tables        []string
}

func queryRows(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]row, []json.RawMessage, error) {
	rs, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	out := []row{}
	raws := []json.RawMessage{}
	for rs.Next() {
		var raw []byte
		if err = rs.Scan(&raw); err != nil {
			return nil, nil, err
		}
		if len(raw) > 2<<20 {
			return nil, nil, fmt.Errorf("single legacy row exceeds bounded2MiB")
		}
		var value row
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, nil, err
		}
		out = append(out, value)
		raws = append(raws, append(json.RawMessage{}, raw...))
		if len(out) > 256 {
			return nil, nil, fmt.Errorf("legacy exact row bound exceeded")
		}
	}
	return out, raws, rs.Err()
}
func quoted(schema, table string) string { return pgx.Identifier{schema, table}.Sanitize() }
func databaseIdentity(ctx context.Context, tx pgx.Tx) (DatabaseIdentity, error) {
	var id DatabaseIdentity
	err := tx.QueryRow(ctx, `SELECT current_database(),current_schema(),current_user,coalesce(host(inet_server_addr()),''),coalesce(inet_server_port(),0),(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&id.Database, &id.Schema, &id.Role, &id.ServerAddress, &id.Port, &id.DatabaseOID)
	return id, err
}
func readCatalog(ctx context.Context, tx pgx.Tx, schema string, tables []string) (actualCatalog, error) {
	c := actualCatalog{PrimaryKeys: map[string][]string{}, TriggerBodies: map[string]string{}}
	classes, err := tx.Query(ctx, `SELECT cl.relname,cl.oid::text,pg_get_userbyid(cl.relowner),cl.relkind::text,cl.relrowsecurity,cl.relforcerowsecurity,cl.relispartition,EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=cl.oid OR i.inhparent=cl.oid) FROM pg_class cl JOIN pg_namespace n ON n.oid=cl.relnamespace WHERE n.nspname=$1 AND cl.relname=ANY($2::text[]) ORDER BY cl.relname`, schema, tables)
	if err != nil {
		return c, err
	}
	for classes.Next() {
		var name, oid, owner, kind string
		var rls, force, partition, inherited bool
		if err = classes.Scan(&name, &oid, &owner, &kind, &rls, &force, &partition, &inherited); err != nil {
			classes.Close()
			return c, err
		}
		if kind != "r" || rls || force || partition || inherited {
			classes.Close()
			return c, fmt.Errorf("unexpected table kind, RLS or inheritance")
		}
		c.Tables = append(c.Tables, name+":"+oid+":"+owner)
	}
	err = classes.Err()
	classes.Close()
	if err != nil {
		return c, err
	}
	if len(c.Tables) != len(tables) {
		return c, fmt.Errorf("fixed table set missing")
	}
	rs, err := tx.Query(ctx, `SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema=$1 AND table_name=ANY($2::text[]) ORDER BY table_name,ordinal_position`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var x column
		if err = rs.Scan(&x.Table, &x.Column, &x.Type); err != nil {
			rs.Close()
			return c, err
		}
		c.Columns = append(c.Columns, x)
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return c, err
	}
	rs, err = tx.Query(ctx, `SELECT con.conname,child.relname,parent.relname,pg_get_constraintdef(con.oid) FROM pg_constraint con JOIN pg_class child ON child.oid=con.conrelid JOIN pg_namespace cn ON cn.oid=child.relnamespace JOIN pg_class parent ON parent.oid=con.confrelid JOIN pg_namespace pn ON pn.oid=parent.relnamespace WHERE con.contype='f' AND ((cn.nspname=$1 AND child.relname=ANY($2::text[])) OR (pn.nspname=$1 AND parent.relname=ANY($2::text[]))) ORDER BY con.conname`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var x foreignKey
		if err = rs.Scan(&x.Name, &x.Child, &x.Parent, &x.Definition); err != nil {
			rs.Close()
			return c, err
		}
		c.ForeignKeys = append(c.ForeignKeys, x)
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return c, err
	}
	rs, err = tx.Query(ctx, `SELECT cl.relname,array_agg(a.attname ORDER BY key.ordinality)::text[] FROM pg_constraint co JOIN pg_class cl ON cl.oid=co.conrelid JOIN pg_namespace n ON n.oid=cl.relnamespace CROSS JOIN LATERAL unnest(co.conkey) WITH ORDINALITY key(num,ordinality) JOIN pg_attribute a ON a.attrelid=cl.oid AND a.attnum=key.num WHERE co.contype='p' AND n.nspname=$1 AND cl.relname=ANY($2::text[]) GROUP BY cl.relname`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var table string
		var keys []string
		if err = rs.Scan(&table, &keys); err != nil {
			rs.Close()
			return c, err
		}
		c.PrimaryKeys[table] = keys
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return c, err
	}
	rs, err = tx.Query(ctx, `SELECT t.tgname,cl.relname,pg_get_triggerdef(t.oid)||'|'||t.tgenabled::text||'|'||p.prosecdef::text,p.prosrc FROM pg_trigger t JOIN pg_class cl ON cl.oid=t.tgrelid JOIN pg_namespace n ON n.oid=cl.relnamespace JOIN pg_proc p ON p.oid=t.tgfoid WHERE NOT t.tgisinternal AND n.nspname=$1 AND cl.relname=ANY($2::text[]) ORDER BY cl.relname,t.tgname`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var x trigger
		var body string
		if err = rs.Scan(&x.Name, &x.Table, &x.Definition, &body); err != nil {
			rs.Close()
			return c, err
		}
		c.Triggers = append(c.Triggers, x)
		c.TriggerBodies[x.Name] = bytesSHA([]byte(strings.TrimSpace(body)))
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return c, err
	}
	rs, err = tx.Query(ctx, `SELECT cl.relname||':'||co.conname||':'||pg_get_constraintdef(co.oid)||':'||co.convalidated::text FROM pg_constraint co JOIN pg_class cl ON cl.oid=co.conrelid JOIN pg_namespace n ON n.oid=cl.relnamespace WHERE n.nspname=$1 AND cl.relname=ANY($2::text[]) ORDER BY cl.relname,co.conname`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var x string
		if err = rs.Scan(&x); err != nil {
			rs.Close()
			return c, err
		}
		c.Constraints = append(c.Constraints, x)
	}
	err = rs.Err()
	rs.Close()
	if err != nil {
		return c, err
	}
	rs, err = tx.Query(ctx, `SELECT tablename||':'||rulename||':'||definition FROM pg_rules WHERE schemaname=$1 AND tablename=ANY($2::text[]) ORDER BY tablename,rulename`, schema, tables)
	if err != nil {
		return c, err
	}
	for rs.Next() {
		var x string
		if err = rs.Scan(&x); err != nil {
			rs.Close()
			return c, err
		}
		c.Rules = append(c.Rules, x)
	}
	err = rs.Err()
	rs.Close()
	return c, err
}
func sameCatalogColumns(a, b []column) bool {
	key := func(x column) string { return x.Table + "/" + x.Column + "/" + x.Type }
	aa := []string{}
	bb := []string{}
	for _, x := range a {
		aa = append(aa, key(x))
	}
	for _, x := range b {
		bb = append(bb, key(x))
	}
	sort.Strings(aa)
	sort.Strings(bb)
	return digest(aa) == digest(bb)
}
func checkCatalog(c actualCatalog, md bool) error {
	contract := fixedContract()
	columns, keys := contract.GovColumns, contract.GovKeys
	tables := append(append([]string{}, govOrder...), "sys_quota_accounts", "sys_quota_definitions", "sys_modeldev_release_bindings")
	if md {
		columns, keys = contract.MDColumns, contract.MDKeys
		tables = mdOrder
	}
	if !sameCatalogColumns(c.Columns, columns) {
		return fmt.Errorf("live columns differ from original/code baseline")
	}
	selectKeys := func(all []foreignKey) []foreignKey {
		out := []foreignKey{}
		for _, fk := range all {
			for _, table := range tables {
				if fk.Child == table || fk.Parent == table {
					out = append(out, fk)
					break
				}
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	if digest(selectKeys(c.ForeignKeys)) != digest(selectKeys(keys)) {
		return fmt.Errorf("unknown or changed foreign-key reference")
	}
	expected := map[string][]string{}
	if md {
		expected = contract.PrimaryKeys
	} else {
		for _, table := range tables {
			expected[table] = []string{"id"}
		}
	}
	if digest(c.PrimaryKeys) != digest(expected) {
		return fmt.Errorf("original primary keys differ")
	}
	if len(c.Rules) != 0 {
		return fmt.Errorf("unknown table rewrite rule")
	}
	if !md {
		if len(c.Triggers) != 0 {
			return fmt.Errorf("unknown Governance trigger")
		}
		return nil
	}
	if len(c.Triggers) != 1 || c.Triggers[0].Name != "modeldev_gpu_release_scope_immutable" || c.Triggers[0].Table != "modeldev_execution_runtimes" || c.TriggerBodies[c.Triggers[0].Name] != contract.TriggerBodySHA256 {
		return fmt.Errorf("original immutable GPU trigger differs")
	}
	// Schema qualification is deployment-local; the entire actual trigger shape,
	// enabled state and function body must otherwise match the code baseline.
	actual := c.Triggers[0].Definition
	for _, part := range strings.Fields(actual) {
		if strings.Contains(part, ".modeldev_execution_runtimes") {
			actual = strings.ReplaceAll(actual, strings.Split(part, ".")[0]+".", "")
		}
	}
	expectedTrigger := strings.ReplaceAll(contract.Triggers[0].Definition, "public.", "") + "|O|false"
	if actual != expectedTrigger {
		return fmt.Errorf("original immutable GPU trigger definition differs")
	}
	return nil
}
func snapshotDatabase(ctx context.Context, tx pgx.Tx, s OriginalScope, md bool) (databaseView, error) {
	v := databaseView{}
	var err error
	v.Snapshot.Identity, err = databaseIdentity(ctx, tx)
	if err != nil {
		return v, err
	}
	if err = tx.QueryRow(ctx, "SELECT session_user").Scan(&v.Snapshot.AuthenticatedRole); err != nil {
		return v, err
	}
	schema := v.Snapshot.Identity.Schema
	tables := append(append([]string{}, govOrder...), "sys_quota_accounts", "sys_quota_definitions", "sys_modeldev_release_bindings")
	if md {
		tables = mdOrder
	}
	catalog, err := readCatalog(ctx, tx, schema, tables)
	if err != nil {
		return v, err
	}
	v.Catalog = catalog
	v.Snapshot.CatalogSHA256 = digest(catalog)
	v.Snapshot.Catalog = &catalog
	if err = checkCatalog(catalog, md); err != nil {
		v.Snapshot.Refusal = err.Error()
		return v, err
	}
	execs := []string{}
	for _, x := range s.Executions {
		execs = append(execs, x.ExecutionID)
	}
	order := govOrder
	if md {
		order = mdOrder
	}
	for _, table := range order {
		var where string
		var args []any
		if md {
			where = "tenant_id=$1::uuid AND execution_id=ANY($2::uuid[])"
			args = []any{s.TenantID, execs}
			if table == "modeldev_input_versions" {
				where = "tenant_id=$1::uuid AND input_version_id=ANY($2::uuid[])"
				args = []any{s.TenantID, s.InputIDs}
			}
		} else {
			where = "tenant_id=$1 AND id=ANY($2::bigint[])"
			ids := s.AcceptanceIDs
			switch table {
			case "sys_quota_operations":
				ids = s.OperationIDs
			case "sys_quota_charges":
				ids = s.ChargeIDs
			case "sys_gpu_usage_sync":
				ids = s.SyncIDs
			case "sys_quota_release_receipts":
				where = "tenant_id=$1 AND receipt_id=ANY($2::text[])"
				args = []any{s.GovernanceTenantID, s.ReceiptIDs}
			}
			if args == nil {
				args = []any{s.GovernanceTenantID, ids}
			}
		}
		rows, raw, err := queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, table)+" t WHERE "+where+" ORDER BY to_jsonb(t)::text", args...)
		if err != nil {
			return v, err
		}
		tr := tableRows{table, rows, raw}
		v.Tables = append(v.Tables, tr)
		summary := TableSummary{Table: table, Rows: []RowSummary{}}
		keys := catalog.PrimaryKeys[table]
		for i, r := range rows {
			values := []string{}
			for _, key := range keys {
				values = append(values, key+"="+text(r, key))
			}
			summary.Rows = append(summary.Rows, RowSummary{strings.Join(values, "/"), bytesSHA(raw[i])})
		}
		v.Snapshot.Tables = append(v.Snapshot.Tables, summary)
	}
	if !md {
		v.Accounts, _, err = queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, "sys_quota_accounts")+" t WHERE tenant_id=$1 ORDER BY id", s.GovernanceTenantID)
		if err != nil {
			return v, err
		}
		v.Snapshot.AccountsSHA256 = digest(v.Accounts)
		// Existing GPU-owner incoming FKs are retained, never recursively deleted.
		// Bind zero-reference proofs to the exact original operation IDs, not to
		// global row counts or an owner label.
		ids := []string{}
		for _, original := range originalRows(s.OriginalOperations) {
			ids = append(ids, text(original, "operation_id"))
		}
		sort.Strings(ids)
		v.Snapshot.IncomingOperationScopeSHA256 = digest(struct {
			TenantID     int64
			OperationIDs []string
		}{s.GovernanceTenantID, ids})
		for _, check := range []struct{ Table, Predicate string }{
			{"sys_gpu_delete_acceptances", "create_operation_id=ANY($2::text[]) OR delete_operation_id=ANY($2::text[])"},
			{"sys_gpu_usage_sync", "operation_id=ANY($2::text[])"},
		} {
			references, raw, queryErr := queryRows(ctx, tx, "SELECT to_jsonb(t) FROM "+quoted(schema, check.Table)+" t WHERE tenant_id=$1 AND ("+check.Predicate+") ORDER BY id", s.GovernanceTenantID, ids)
			if queryErr != nil {
				return v, queryErr
			}
			summary := TableSummary{Table: check.Table, Rows: []RowSummary{}}
			for i, reference := range references {
				summary.Rows = append(summary.Rows, RowSummary{Key: "id=" + text(reference, "id"), SHA256: bytesSHA(raw[i])})
			}
			v.Snapshot.IncomingReferences = append(v.Snapshot.IncomingReferences, summary)
		}
	}
	return v, nil
}
func lockTables(ctx context.Context, tx pgx.Tx, md bool) error {
	identity, err := databaseIdentity(ctx, tx)
	if err != nil {
		return err
	}
	tables := append(append([]string{}, govOrder...), "sys_quota_accounts", "sys_modeldev_release_bindings", "sys_gpu_delete_acceptances")
	if md {
		tables = append([]string{}, mdOrder...)
	}
	sort.Strings(tables)
	names := []string{}
	for _, table := range tables {
		names = append(names, quoted(identity.Schema, table))
	}
	_, err = tx.Exec(ctx, "LOCK TABLE "+strings.Join(names, ",")+" IN SHARE ROW EXCLUSIVE MODE")
	return err
}
func deleteExactRows(ctx context.Context, tx pgx.Tx, v databaseView) (map[string]int64, error) {
	deleted := map[string]int64{}
	catalog := v.Catalog.(actualCatalog)
	for _, table := range v.Tables {
		deleted[table.table] = 0
		for i, r := range table.rows {
			args := []any{[]byte(table.raw[i]), text(r, "tenant_id")}
			where := []string{"to_jsonb(t)=$1::jsonb", "tenant_id::text=$2"}
			for _, key := range catalog.PrimaryKeys[table.table] {
				args = append(args, text(r, key))
				where = append(where, pgx.Identifier{key}.Sanitize()+fmt.Sprintf("::text=$%d", len(args)))
			}
			tag, err := tx.Exec(ctx, "DELETE FROM "+quoted(v.Snapshot.Identity.Schema, table.table)+" t WHERE "+strings.Join(where, " AND "), args...)
			if err != nil {
				return nil, err
			}
			if tag.RowsAffected() != 1 {
				return nil, fmt.Errorf("exact DELETE expected1 row; transaction rollback")
			}
			deleted[table.table] += tag.RowsAffected()
		}
		if deleted[table.table] != int64(len(table.rows)) {
			return nil, fmt.Errorf("DELETE count differs; transaction rollback")
		}
	}
	return deleted, nil
}
func table(v databaseView, name string) tableRows {
	for _, t := range v.Tables {
		if t.table == name {
			return t
		}
	}
	return tableRows{table: name}
}
