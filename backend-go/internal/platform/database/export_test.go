package database

// export_test.go exposes unexported migration helpers to the external test
// package (database_test) so integration tests can drive them directly with
// seeded data. These vars exist only in the test build.
var (
	ExportedRunAuxLabelDupMerge = runAuxLabelDupMergeHook // hook wrapper (logic moved to tagmanagement/models)

	// RunMigrationsList exposes the core migration loop so tests can run a
	// hand-built migration list (e.g. a probe migration with RunOutsideTx=true)
	// against a testcontainer without mutating the production
	// postgresMigrations() slice. See db-migration-execution capability.
	RunMigrationsList = runMigrationsList

	// WithLockTimeout exposes the lock_timeout guard helper so integration tests
	// can assert its SET LOCAL semantics (GUC applies during fn, resets after).
	WithLockTimeout = withLockTimeout

	// ExportedPostgresMigrations exposes the production migration list so
	// integration tests can locate a single migration by Version and drive its
	// Up closure against a seeded testcontainer (without running the full
	// ~60-migration path). Used by fix-watch-delete-cascade's historical-orphan
	// migration test.
	ExportedPostgresMigrations = postgresMigrations
)
