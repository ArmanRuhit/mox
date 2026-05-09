package main

// cmdPgMigrate migrates data from the bstore-backed databases to PostgreSQL.
// mox must NOT be running when this command is used.

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/mjl-/bstore"

	"github.com/mjl-/mox/mlog"
	"github.com/mjl-/mox/mox-"
	"github.com/mjl-/mox/queue"
	"github.com/mjl-/mox/store"
	"github.com/mjl-/mox/webapi"
)

func cmdPgMigrate(c *cmd) {
	c.help = `Migrate data from the bstore databases to PostgreSQL.

This command reads every record from the existing bstore databases (auth,
queue, and per-account index databases) and writes them into the PostgreSQL
instance configured under the PostgreSQL block in mox.conf. It is a one-time,
offline migration: mox must not be running while this command executes.

Each type is migrated in a single transaction. If a type fails, that
transaction is rolled back and the error is reported; other types continue.
Use --dry-run to count records without writing anything.

After a successful migration, restart mox with the PostgreSQL block enabled
and verify normal operation before removing the old bstore files.
`
	var dryRun bool
	c.flag.BoolVar(&dryRun, "dry-run", false, "count records only; do not write to PostgreSQL")
	c.Parse()
	mustLoadConfig()

	ctx := context.Background()
	log := mlog.New("pgmigrate", nil)

	pgcfg := mox.Conf.Static.PostgreSQL
	if pgcfg == nil {
		log.Fatal("no PostgreSQL block configured in mox.conf")
	}

	pool, err := store.InitPool(ctx, pgcfg)
	if err != nil {
		log.Fatalx("initialise pg pool", err)
	}
	defer store.ClosePool()

	type result struct {
		kind string
		n    int
		err  error
	}
	var results []result

	report := func(kind string, n int, err error) {
		if err != nil {
			log.Errorx("migrating", err, slog.String("kind", kind))
		} else {
			log.Print("migrated", slog.String("kind", kind), slog.Int("rows", n))
		}
		results = append(results, result{kind, n, err})
	}

	// ── Auth DB ──────────────────────────────────────────────────────────────
	authBstorePath := mox.DataDirPath("auth.db")
	authRaw, err := bstore.Open(ctx, authBstorePath, &bstore.Options{MustExist: true}, store.AuthDBTypes...)
	if err != nil {
		log.Fatalx("open bstore auth.db", err)
	}
	defer authRaw.Close()

	if err := store.EnsureSchema(ctx, pool, "auth", "auth"); err != nil {
		log.Fatalx("ensure auth schema", err)
	}
	authPG := store.NewPgDB(pool, "auth")
	defer authPG.Close()

	n, err := migrateTable[store.TLSPublicKey](ctx, authRaw, authPG, dryRun)
	report("auth/TLSPublicKey", n, err)
	n, err = migrateTable[store.LoginAttempt](ctx, authRaw, authPG, dryRun)
	report("auth/LoginAttempt", n, err)
	n, err = migrateTable[store.LoginAttemptState](ctx, authRaw, authPG, dryRun)
	report("auth/LoginAttemptState", n, err)
	n, err = migrateTable[store.AccountRemove](ctx, authRaw, authPG, dryRun)
	report("auth/AccountRemove", n, err)

	// ── Queue DB ─────────────────────────────────────────────────────────────
	queueBstorePath := mox.DataDirPath("queue/index.db")
	queueRaw, err := bstore.Open(ctx, queueBstorePath, &bstore.Options{MustExist: true}, queue.DBTypes...)
	if err != nil {
		log.Fatalx("open bstore queue/index.db", err)
	}
	defer queueRaw.Close()

	if err := store.EnsureSchema(ctx, pool, "queue", "queue"); err != nil {
		log.Fatalx("ensure queue schema", err)
	}
	queuePG := store.NewPgDB(pool, "queue")
	defer queuePG.Close()

	n, err = migrateTable[queue.Msg](ctx, queueRaw, queuePG, dryRun)
	report("queue/Msg", n, err)
	n, err = migrateTable[queue.HoldRule](ctx, queueRaw, queuePG, dryRun)
	report("queue/HoldRule", n, err)
	n, err = migrateTable[queue.MsgRetired](ctx, queueRaw, queuePG, dryRun)
	report("queue/MsgRetired", n, err)
	n, err = migrateTable[queue.Hook](ctx, queueRaw, queuePG, dryRun)
	report("queue/Hook", n, err)
	n, err = migrateTable[queue.HookRetired](ctx, queueRaw, queuePG, dryRun)
	report("queue/HookRetired", n, err)
	n, err = migrateTable[webapi.Suppression](ctx, queueRaw, queuePG, dryRun)
	report("queue/Suppression", n, err)

	// ── Per-account DBs ──────────────────────────────────────────────────────
	accountNames := mox.Conf.Accounts()
	for _, accountName := range accountNames {
		accountDir := mox.DataDirPath("accounts/" + accountName)
		dbPath := accountDir + "/index.db"

		if _, err := os.Stat(dbPath); err != nil {
			if os.IsNotExist(err) {
				log.Print("no index.db for account, skipping", slog.String("account", accountName))
				continue
			}
			report("account/"+accountName, 0, fmt.Errorf("stat index.db: %w", err))
			continue
		}

		accRaw, err := bstore.Open(ctx, dbPath, &bstore.Options{MustExist: true}, store.DBTypes...)
		if err != nil {
			report("account/"+accountName, 0, fmt.Errorf("open bstore: %w", err))
			continue
		}

		schemaName := "account_" + accountName
		if err := store.EnsureSchema(ctx, pool, schemaName, "account"); err != nil {
			accRaw.Close()
			report("account/"+accountName, 0, fmt.Errorf("ensure schema: %w", err))
			continue
		}
		accPG := store.NewPgDB(pool, schemaName)

		migrateAccount(ctx, log, accRaw, accPG, accountName, dryRun, report)

		accPG.Close()
		accRaw.Close()
	}

	// ── Summary ───────────────────────────────────────────────────────────────
	var totalRows, failedKinds int
	for _, r := range results {
		totalRows += r.n
		if r.err != nil {
			failedKinds++
		}
	}
	if dryRun {
		log.Print("dry run complete", slog.Int("total_rows", totalRows), slog.Int("types", len(results)))
	} else if failedKinds > 0 {
		log.Print("migration complete with errors", slog.Int("total_rows", totalRows), slog.Int("failed_types", failedKinds))
		os.Exit(1)
	} else {
		log.Print("migration complete", slog.Int("total_rows", totalRows), slog.Int("types", len(results)))
	}
}

// migrateTable reads all rows of T from src (bstore) and inserts them into
// dst (PG) inside a single transaction. Returns the row count.
func migrateTable[T any](ctx context.Context, src *bstore.DB, dst store.DB, dryRun bool) (int, error) {
	rows, err := bstore.QueryDB[T](ctx, src).List()
	if err != nil {
		return 0, fmt.Errorf("list from bstore: %w", err)
	}
	if dryRun {
		return len(rows), nil
	}
	if len(rows) == 0 {
		return 0, nil
	}
	err = dst.Write(ctx, func(tx store.Tx) error {
		for i := range rows {
			if err := tx.Insert(&rows[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("insert into pg: %w", err)
	}
	return len(rows), nil
}

// migrateAccount migrates all per-account types using migrateTable.
func migrateAccount(ctx context.Context, log mlog.Log, src *bstore.DB, dst store.DB, accountName string, dryRun bool, report func(string, int, error)) {
	prefix := "account/" + accountName + "/"

	n, err := migrateTable[store.NextUIDValidity](ctx, src, dst, dryRun)
	report(prefix+"NextUIDValidity", n, err)
	n, err = migrateTable[store.SyncState](ctx, src, dst, dryRun)
	report(prefix+"SyncState", n, err)
	n, err = migrateTable[store.DiskUsage](ctx, src, dst, dryRun)
	report(prefix+"DiskUsage", n, err)
	n, err = migrateTable[store.Settings](ctx, src, dst, dryRun)
	report(prefix+"Settings", n, err)
	n, err = migrateTable[store.Upgrade](ctx, src, dst, dryRun)
	report(prefix+"Upgrade", n, err)
	n, err = migrateTable[store.Subjectpass](ctx, src, dst, dryRun)
	report(prefix+"Subjectpass", n, err)
	n, err = migrateTable[store.RecipientDomainTLS](ctx, src, dst, dryRun)
	report(prefix+"RecipientDomainTLS", n, err)
	n, err = migrateTable[store.FromAddressSettings](ctx, src, dst, dryRun)
	report(prefix+"FromAddressSettings", n, err)
	n, err = migrateTable[store.Subscription](ctx, src, dst, dryRun)
	report(prefix+"Subscription", n, err)
	n, err = migrateTable[store.Outgoing](ctx, src, dst, dryRun)
	report(prefix+"Outgoing", n, err)
	n, err = migrateTable[store.MessageErase](ctx, src, dst, dryRun)
	report(prefix+"MessageErase", n, err)
	n, err = migrateTable[store.RulesetNoListID](ctx, src, dst, dryRun)
	report(prefix+"RulesetNoListID", n, err)
	n, err = migrateTable[store.RulesetNoMsgFrom](ctx, src, dst, dryRun)
	report(prefix+"RulesetNoMsgFrom", n, err)
	n, err = migrateTable[store.RulesetNoMailbox](ctx, src, dst, dryRun)
	report(prefix+"RulesetNoMailbox", n, err)
	n, err = migrateTable[store.LoginSession](ctx, src, dst, dryRun)
	report(prefix+"LoginSession", n, err)
	n, err = migrateTable[store.Recipient](ctx, src, dst, dryRun)
	report(prefix+"Recipient", n, err)
	n, err = migrateTable[store.Annotation](ctx, src, dst, dryRun)
	report(prefix+"Annotation", n, err)
	n, err = migrateTable[store.Password](ctx, src, dst, dryRun)
	report(prefix+"Password", n, err)
	n, err = migrateTable[store.Mailbox](ctx, src, dst, dryRun)
	report(prefix+"Mailbox", n, err)
	n, err = migrateTable[store.Message](ctx, src, dst, dryRun)
	report(prefix+"Message", n, err)
}
