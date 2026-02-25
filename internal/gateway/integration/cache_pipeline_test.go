package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	_ "github.com/jackc/pgx/v5/stdlib"

	insightifyv1 "insightify/gen/go/insightify/v1"
	uicache "insightify/internal/cache/ui"
	uiworkspacecache "insightify/internal/cache/uiworkspace"
	"insightify/internal/gateway/ent"
	uirepo "insightify/internal/gateway/repository/ui"
	uiworkspacerepo "insightify/internal/gateway/repository/uiworkspace"
	gatewayui "insightify/internal/gateway/service/ui"
	gatewayuiworkspace "insightify/internal/gateway/service/uiworkspace"
)

func TestCachePipeline_DBBackedRestoreAcrossRestart(t *testing.T) {
	if strings.TrimSpace(os.Getenv("RUN_DB_E2E")) != "1" {
		t.Skip("set RUN_DB_E2E=1 to run DB integration tests")
	}

	dsn := testDSN()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	db1, client1 := openClient(t, dsn)
	resetSchemaState(t, db1)

	projectID := fmt.Sprintf("project-e2e-%d", time.Now().UnixNano())
	runID := fmt.Sprintf("run-%s-%d", projectID, time.Now().UnixNano())

	workspaceSvc1, uiSvc1 := newServices(client1)

	tab, err := workspaceSvc1.AttachRunToCurrentTab(projectID, runID)
	if err != nil {
		t.Fatalf("attach run to tab: %v", err)
	}

	applyRes, err := uiSvc1.ApplyOps(ctx, &insightifyv1.ApplyUiOpsRequest{
		RunId:       runID,
		BaseVersion: 0,
		Ops: []*insightifyv1.UiOp{
			{
				Action: &insightifyv1.UiOp_UpsertNode{
					UpsertNode: &insightifyv1.UiUpsertNode{
						Node: &insightifyv1.UiNode{
							Id:   "node-db-e2e-1",
							Type: insightifyv1.UiNodeType_UI_NODE_TYPE_MARKDOWN,
							Markdown: &insightifyv1.UiMarkdownState{
								Markdown: "db integration node",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("apply ops: %v", err)
	}
	if applyRes.GetConflict() {
		t.Fatalf("unexpected conflict on first write")
	}

	var (
		tabRows int
		uiRows  int
	)
	if err := db1.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspace_tabs WHERE tab_id = $1 AND run_id = $2", tab.TabID, runID).Scan(&tabRows); err != nil {
		t.Fatalf("query workspace_tabs: %v", err)
	}
	if err := db1.QueryRowContext(ctx, "SELECT COUNT(*) FROM user_interactions WHERE run_id = $1", runID).Scan(&uiRows); err != nil {
		t.Fatalf("query user_interactions: %v", err)
	}
	if tabRows != 1 || uiRows != 1 {
		t.Fatalf("unexpected row counts tabRows=%d uiRows=%d", tabRows, uiRows)
	}

	// Simulate process restart with fresh DB connection and empty in-memory caches.
	client1.Close()
	_ = db1.Close()

	_, client2 := openClient(t, dsn)
	_, uiSvc2 := newServices(client2)

	restoreRes, err := uiSvc2.Restore(ctx, &insightifyv1.RestoreUiRequest{
		ProjectId: projectID,
		TabId:     tab.TabID,
	})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restoreRes.GetReason() != insightifyv1.UiRestoreReason_UI_RESTORE_REASON_RESOLVED {
		t.Fatalf("unexpected restore reason: %v", restoreRes.GetReason())
	}
	if strings.TrimSpace(restoreRes.GetRunId()) != runID {
		t.Fatalf("unexpected run_id: got=%q want=%q", restoreRes.GetRunId(), runID)
	}
	doc := restoreRes.GetDocument()
	if doc == nil {
		t.Fatalf("expected document in restore response")
	}
	if len(doc.GetNodes()) != 1 {
		t.Fatalf("unexpected node count: got=%d want=1", len(doc.GetNodes()))
	}
	if got := strings.TrimSpace(doc.GetNodes()[0].GetId()); got != "node-db-e2e-1" {
		t.Fatalf("unexpected node id: got=%q", got)
	}
}

func openClient(t *testing.T, dsn string) (*sql.DB, *ent.Client) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}
	return db, client
}

func newServices(client *ent.Client) (*gatewayuiworkspace.Service, *gatewayui.Service) {
	uiStore := uicache.NewCachedStore(uirepo.NewPostgresStore(client), uicache.DefaultCacheConfig())
	uiWorkspaceStore := uiworkspacecache.NewCachedStore(
		uiworkspacerepo.NewPostgresStore(client),
		uiworkspacecache.DefaultCacheConfig(),
	)
	workspaceSvc := gatewayuiworkspace.New(uiWorkspaceStore)
	uiSvc := gatewayui.New(uiStore, workspaceSvc, nil, "")
	return workspaceSvc, uiSvc
}

func resetSchemaState(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(
		ctx,
		"TRUNCATE TABLE workspace_tabs, workspaces, user_interactions, artifact_files, artifacts, projects RESTART IDENTITY CASCADE",
	); err != nil {
		t.Fatalf("reset schema state: %v", err)
	}
}

func testDSN() string {
	if dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")); dsn != "" {
		return dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URL")); dsn != "" {
		return dsn
	}
	return "postgres://insightify:insightify@localhost:5432/insightify?sslmode=disable"
}
