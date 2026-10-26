package workjournal

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*Service, Scope) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if e = db.AutoMigrate(&Record{}, &RecordVersion{}, &Operation{}); e != nil {
		t.Fatal(e)
	}
	s, e := New(db, time.Now)
	if e != nil {
		t.Fatal(e)
	}
	return s, Scope{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
}
func TestGenericRecordsHistoryCASIsolation(t *testing.T) {
	s, sc := fixture(t)
	ctx := context.Background()
	for _, kind := range []string{"support.ticket", "social.draft"} {
		req := SaveRecordRequest{Kind: kind, Key: "item-1", State: "awaiting-editor", Content: datatypes.JSON(`{"text":"hello"}`)}
		r, e := s.SaveRecord(ctx, sc, req)
		if e != nil || r.Revision != 1 {
			t.Fatalf("create: %+v %v", r, e)
		}
		req.ExpectedRevision = 1
		req.State = "custom-approved"
		r, e = s.SaveRecord(ctx, sc, req)
		if e != nil || r.Revision != 2 {
			t.Fatalf("update: %+v %v", r, e)
		}
		if _, e = s.SaveRecord(ctx, sc, req); e == nil {
			t.Fatal("stale CAS accepted")
		}
		versions, e := s.ListRecordVersions(ctx, sc, r.ID, 0, 20)
		if e != nil || len(versions) != 2 || versions[0].State != "awaiting-editor" {
			t.Fatalf("history %+v %v", versions, e)
		}
		foreign := sc
		foreign.OwnerID = uuid.New()
		if _, e = s.GetRecord(ctx, foreign, r.ID); e == nil {
			t.Fatal("foreign record visible")
		}
		if _, e = s.GetRecordByKey(ctx, sc, kind, "item-1"); e != nil {
			t.Fatal(e)
		}
	}
	rows, e := s.ListRecords(ctx, sc, "", uuid.Nil, 1)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	next, e := s.ListRecords(ctx, sc, "", rows[0].ID, 20)
	if e != nil || len(next) != 1 {
		t.Fatal(next, e)
	}
}

type gateway struct {
	calls      atomic.Int32
	recoveries atomic.Int32
	result     DispatchResult
	err        error
	cancel     context.CancelFunc
}

func (g *gateway) Execute(context.Context, Operation) (DispatchResult, error) {
	g.calls.Add(1)
	if g.cancel != nil {
		g.cancel()
	}
	return g.result, g.err
}
func (g *gateway) Recover(context.Context, Operation) (DispatchResult, error) {
	g.recoveries.Add(1)
	return g.result, g.err
}
func request() PrepareRequest {
	return PrepareRequest{WorkKey: "work-1", EmployeeVersionID: uuid.New(), ConnectorID: "arbitrary-mail", ConnectionID: uuid.New(), OperationID: "send_message", Input: datatypes.JSON(`{"to":"a","body":"b"}`)}
}
func TestOperationIdempotencyFingerprintConcurrencyAndRecovery(t *testing.T) {
	s, sc := fixture(t)
	ctx := context.Background()
	req := request()
	op, e := s.Prepare(ctx, sc, req)
	if e != nil {
		t.Fatal(e)
	}
	req.Input = datatypes.JSON(`{ "body":"b", "to":"a" }`)
	same, e := s.Prepare(ctx, sc, req)
	if e != nil || same.ID != op.ID {
		t.Fatal(same, e)
	}
	req.Input = datatypes.JSON(`{"different":true}`)
	if _, e = s.Prepare(ctx, sc, req); e == nil {
		t.Fatal("changed input accepted")
	}
	req = request()
	req.WorkKey = "bypass"
	req.EmployeeVersionID = uuid.New()
	req.ConnectionID = op.ConnectionID
	if _, e = s.Prepare(ctx, sc, req); e == nil {
		t.Fatal("unresolved duplicate bypassed")
	}
	g := &gateway{err: errors.New("uncertain")}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.Dispatch(ctx, sc, op.ID, g) }()
	}
	wg.Wait()
	if g.calls.Load() != 1 {
		t.Fatalf("calls %d", g.calls.Load())
	}
	op, e = s.GetOperation(ctx, sc, op.ID)
	if e != nil || op.State != StateUnknown || op.Attempts != 1 {
		t.Fatal(op, e)
	}
	restored, _ := New(s.db, s.now)
	_, _ = restored.Dispatch(ctx, sc, op.ID, g)
	if g.calls.Load() != 1 {
		t.Fatal("resent after restart")
	}
	g.err = nil
	g.result = DispatchResult{State: StatePending}
	op, e = s.Recover(ctx, sc, op.ID, g)
	if e != nil || op.State != StateUnknown {
		t.Fatal("invalid gateway transition", op, e)
	}
	g.result = DispatchResult{State: StateSucceeded, Receipt: datatypes.JSON(`{"accepted":true}`)}
	op, e = s.Recover(ctx, sc, op.ID, g)
	if e != nil || op.State != StateSucceeded {
		t.Fatal(op, e)
	}
	before := g.recoveries.Load()
	_, _ = s.Recover(ctx, sc, op.ID, g)
	if g.recoveries.Load() != before || g.calls.Load() != 1 {
		t.Fatal("terminal recovery or resend")
	}
	foreign := sc
	foreign.EmployeeID = uuid.New()
	if _, e = s.Dispatch(ctx, foreign, op.ID, g); e == nil {
		t.Fatal("foreign dispatch")
	}
	if op.LogicalID() == "" || len(op.LogicalID()) > 128 {
		t.Fatal(op.LogicalID())
	}
}
func TestCancelledRemoteResultStillPersistsAndCrashRecovery(t *testing.T) {
	s, sc := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	op, e := s.Prepare(ctx, sc, request())
	if e != nil {
		t.Fatal(e)
	}
	g := &gateway{cancel: cancel, result: DispatchResult{State: StateSucceeded, Receipt: datatypes.JSON(`{"submitted":true}`)}}
	op, e = s.Dispatch(ctx, sc, op.ID, g)
	if e != nil || op.State != StateSucceeded {
		t.Fatal(op, e)
	}
	req := request()
	req.WorkKey = "second"
	op, e = s.Prepare(context.Background(), sc, req)
	if e != nil {
		t.Fatal(e)
	}
	s.db.Model(&Operation{}).Where("id = ?", op.ID).Updates(map[string]any{"state": StateExecuting, "attempts": 1})
	g.result = DispatchResult{State: StateFailed}
	op, e = s.Recover(context.Background(), sc, op.ID, g)
	if e != nil || op.State != StateFailed || g.calls.Load() != 1 {
		t.Fatal(op, e)
	}
}

func TestValidationAndConcurrentPrepare(t *testing.T) {
	s, sc := fixture(t)
	ctx := context.Background()
	for _, input := range []string{`[]`, `null`, `{"x":1} {}`, `{"invalid":`, strings.Repeat("x", MaxPayloadBytes+1)} {
		req := request()
		req.Input = []byte(input)
		if _, e := s.Prepare(ctx, sc, req); e == nil {
			t.Fatalf("accepted input %q", input[:min(len(input), 20)])
		}
	}
	for _, change := range []func(*PrepareRequest){func(r *PrepareRequest) { r.WorkKey = "" }, func(r *PrepareRequest) { r.WorkKey = strings.Repeat("a", 129) }, func(r *PrepareRequest) { r.OperationID = "bad\nvalue" }, func(r *PrepareRequest) { r.EmployeeVersionID = uuid.Nil }, func(r *PrepareRequest) { r.ConnectionID = uuid.Nil }} {
		req := request()
		change(&req)
		if _, e := s.Prepare(ctx, sc, req); e == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if _, e := s.Prepare(ctx, Scope{}, request()); e == nil {
		t.Fatal("empty scope accepted")
	}
	if _, e := s.ListOperations(ctx, sc, "made-up", uuid.Nil, 10); e == nil {
		t.Fatal("invalid operation state accepted")
	}
	req := request()
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			op, e := s.Prepare(ctx, sc, req)
			if e != nil {
				t.Error(e)
				return
			}
			ids <- op.ID
		}()
	}
	wg.Wait()
	close(ids)
	var first uuid.UUID
	for id := range ids {
		if first == uuid.Nil {
			first = id
		}
		if id != first {
			t.Fatal("duplicate preparation")
		}
	}
	rows, e := s.ListOperations(ctx, sc, StatePending, uuid.Nil, 1)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	g := &gateway{result: DispatchResult{State: StateFailed, Receipt: datatypes.JSON(`{"rejected":true}`)}, err: errors.New("classified rejection")}
	op, e := s.Dispatch(ctx, sc, first, g)
	if e != nil || op.State != StateFailed {
		t.Fatal(op, e)
	}
	g.result.State = StateSucceeded
	_, _ = s.Dispatch(ctx, sc, first, g)
	_, _ = s.Recover(ctx, sc, first, g)
	if g.calls.Load() != 1 || g.recoveries.Load() != 0 {
		t.Fatal("failed terminal resubmitted")
	}
}
func TestPersistedJournalSurvivesReopenedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.db")
	open := func() *gorm.DB {
		db, e := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if e != nil {
			t.Fatal(e)
		}
		return db
	}
	db := open()
	if e := db.AutoMigrate(&Record{}, &RecordVersion{}, &Operation{}); e != nil {
		t.Fatal(e)
	}
	s, _ := New(db, time.Now)
	sc := Scope{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	req := request()
	op, e := s.Prepare(context.Background(), sc, req)
	if e != nil {
		t.Fatal(e)
	}
	g := &gateway{err: errors.New("connection closed after send")}
	op, e = s.Dispatch(context.Background(), sc, op.ID, g)
	if e != nil || op.State != StateUnknown {
		t.Fatal(op, e)
	}
	sql, _ := db.DB()
	sql.Close()
	db = open()
	sql, _ = db.DB()
	defer sql.Close()
	s, _ = New(db, time.Now)
	op, e = s.GetOperationByKey(context.Background(), sc, req.WorkKey)
	if e != nil || op.State != StateUnknown || op.Attempts != 1 || op.EmployeeVersionID != req.EmployeeVersionID {
		t.Fatal(op, e)
	}
	_, _ = s.Dispatch(context.Background(), sc, op.ID, g)
	if g.calls.Load() != 1 {
		t.Fatal("reopen resubmitted")
	}
}

func TestRecordSourcesAreArraysAndRevisionFitsDatabase(t *testing.T) {
	s, sc := fixture(t)
	for _, source := range []string{`null`, `{}`, `true`, `"url"`} {
		req := SaveRecordRequest{Kind: "ticket", Key: "one", State: "open", Content: []byte(`{}`), Sources: []byte(source)}
		if _, e := s.SaveRecord(context.Background(), sc, req); e == nil {
			t.Fatalf("accepted non-array sources %s", source)
		}
	}
	req := SaveRecordRequest{Kind: "ticket", Key: "one", State: "open", Content: []byte(`{}`), ExpectedRevision: math.MaxInt64}
	if _, e := s.SaveRecord(context.Background(), sc, req); e == nil || !isInvalid(e) {
		t.Fatal("overflow revision must be invalid", e)
	}
}
func isInvalid(err error) bool {
	var typed *core.Error
	return errors.As(err, &typed) && typed.Code == core.ErrorCodeInvalidInput
}

func TestNormalizedPayloadRemainsBounded(t *testing.T) {
	s, sc := fixture(t)
	req := request()
	req.Input = datatypes.JSON(`{"text":"` + strings.Repeat("<", MaxPayloadBytes/3) + `"}`)
	if len(req.Input) > MaxPayloadBytes {
		t.Fatal("test input exceeds raw bound")
	}
	if _, err := s.Prepare(context.Background(), sc, req); err == nil {
		t.Fatal("HTML escaping amplification exceeded normalized payload bound")
	}
}

func TestFrozenInputNumericLexemesSurviveReopenAndDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "numeric-journal.db")
	open := func() *gorm.DB {
		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	db := open()
	if err := db.AutoMigrate(&Record{}, &RecordVersion{}, &Operation{}); err != nil {
		t.Fatal(err)
	}
	sc := Scope{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	s, _ := New(db, time.Now)
	req := request()
	req.Input = datatypes.JSON(`{"large":9007199254740993,"exponent":1e0,"fraction":1.2300}`)
	op, err := s.Prepare(context.Background(), sc, req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"exponent":1e0,"fraction":1.2300,"large":9007199254740993}`
	if string(op.Input) != want {
		t.Fatalf("prepare changed numeric input: %s", op.Input)
	}
	sqlDB, _ := db.DB()
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	sqlDB, _ = db.DB()
	defer sqlDB.Close()
	s, _ = New(db, time.Now)
	reloaded, err := s.GetOperation(context.Background(), sc, op.ID)
	if err != nil || string(reloaded.Input) != want || reloaded.Digest != op.Digest {
		t.Fatalf("reload changed frozen input: %+v %v", reloaded, err)
	}
	g := &capturingGateway{}
	reloaded, err = s.Dispatch(context.Background(), sc, op.ID, g)
	if err != nil || reloaded.State != StateSucceeded || string(g.input) != want {
		t.Fatalf("dispatch changed frozen input: %s %+v %v", g.input, reloaded, err)
	}
}

type capturingGateway struct{ input datatypes.JSON }

func (g *capturingGateway) Execute(_ context.Context, op Operation) (DispatchResult, error) {
	g.input = append(datatypes.JSON(nil), op.Input...)
	return DispatchResult{State: StateSucceeded}, nil
}
func (g *capturingGateway) Recover(context.Context, Operation) (DispatchResult, error) {
	panic("unexpected recovery")
}
