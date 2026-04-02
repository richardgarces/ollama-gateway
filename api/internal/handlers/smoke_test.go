package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ollama-gateway/internal/config"
	"ollama-gateway/internal/domain"
	"ollama-gateway/internal/observability"
)

type fakeAgent struct{}

func (fakeAgent) Run(prompt string) string { return "ok" }

type fakeRAGHandler struct{}

func (fakeRAGHandler) GenerateWithContext(prompt string) (string, error) { return "ok", nil }
func (fakeRAGHandler) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	return onChunk("ok")
}

type fakeDebugService struct{}

func (fakeDebugService) AnalyzeError(stackTrace string) (domain.DebugAnalysis, error) {
	return domain.DebugAnalysis{RootCause: "x"}, nil
}
func (fakeDebugService) AnalyzeLog(logLines string) (domain.DebugAnalysis, error) {
	return domain.DebugAnalysis{RootCause: "x"}, nil
}

type fakeTranslator struct{}

func (fakeTranslator) Translate(code, fromLang, toLang string) (string, error) { return "ok", nil }
func (fakeTranslator) TranslateFile(path, toLang string) (string, error)       { return "ok", nil }

type fakeTestGen struct{}

func (fakeTestGen) GenerateTests(code, lang string) (string, error)  { return "ok", nil }
func (fakeTestGen) GenerateTestsForFile(path string) (string, error) { return "ok", nil }
func (fakeTestGen) WriteTestsForFile(sourcePath, testCode string) (string, string, error) {
	return "x_test.go", "", nil
}

type fakeSQL struct{}

func (fakeSQL) GenerateQuery(description, dialect string) (string, error) { return "select 1", nil }
func (fakeSQL) GenerateMigration(description, dialect string) (string, error) {
	return "create table x(id int);", nil
}
func (fakeSQL) ExplainQuery(sql string) (string, error) { return "ok", nil }

type fakeCICD struct{}

func (fakeCICD) GeneratePipeline(platform, repoRoot string) (string, error) { return "name: ci", nil }
func (fakeCICD) OptimizePipeline(existing string, platform string) (string, error) {
	return "name: ci", nil
}
func (fakeCICD) ApplyPipeline(platform, repoRoot, content string) (string, string, error) {
	return ".github/workflows/ci.yml", "", nil
}

type fakeArchitect struct{}

func (fakeArchitect) AnalyzeProject() (domain.ArchReport, error) {
	return domain.ArchReport{Score1To10: 8}, nil
}
func (fakeArchitect) SuggestRefactor(path string) (string, error) { return "ok", nil }

type fakeRepo struct{}

func (fakeRepo) Refactor(absPath string) (string, error) { return "ok", nil }
func (fakeRepo) AnalyzeRepo() (string, error)            { return "ok", nil }

type fakeProfileSvc struct{}

func (fakeProfileSvc) GetByUserID(ctx context.Context, userID string) (*domain.Profile, error) {
	return &domain.Profile{UserID: userID}, nil
}
func (fakeProfileSvc) Upsert(ctx context.Context, profile domain.Profile) (*domain.Profile, error) {
	return &profile, nil
}

type fakePatchSvc struct{}

func (fakePatchSvc) ExtractCodeBlocks(response string) []domain.CodeBlock {
	return []domain.CodeBlock{}
}
func (fakePatchSvc) ExtractDiff(response string) []domain.UnifiedDiff { return []domain.UnifiedDiff{} }
func (fakePatchSvc) ApplyPatch(repoRoot string, diff domain.UnifiedDiff) error {
	return nil
}

type fakeIndexer struct{}

func (fakeIndexer) IndexRepo() error    { return nil }
func (fakeIndexer) StartWatcher() error { return nil }
func (fakeIndexer) StopWatcher()        {}
func (fakeIndexer) ClearState() error   { return nil }
func (fakeIndexer) Status() map[string]interface{} {
	return map[string]interface{}{"indexed_files": 1, "watcher_active": false}
}

type fakeMetrics struct{}

func (fakeMetrics) Snapshot() observability.MetricsSnapshot {
	return observability.MetricsSnapshot{StartedAt: time.Now().UTC()}
}

func req(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

func TestHandlersValidationSmoke(t *testing.T) {
	// agent
	{
		h := NewAgentHandler(fakeAgent{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/agent", "{"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("agent bad body expected 400")
		}
	}
	// generate
	{
		h := NewGenerateHandler(fakeRAGHandler{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/generate", "{"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("generate bad body expected 400")
		}
	}
	// chat
	{
		h := NewChatHandler(fakeRAGHandler{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/v1/chat/completions", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("chat missing messages expected 400")
		}
	}
	// debug
	{
		h := NewDebugHandler(fakeDebugService{})
		w := httptest.NewRecorder()
		h.AnalyzeError(w, req(http.MethodPost, "/api/debug/error", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("debug missing stack expected 400")
		}
	}
	// translator
	{
		h := NewTranslatorHandler(fakeTranslator{})
		w := httptest.NewRecorder()
		h.Translate(w, req(http.MethodPost, "/api/translate", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("translate missing fields expected 400")
		}
	}
	// testgen
	{
		h := NewTestGenHandler(fakeTestGen{})
		w := httptest.NewRecorder()
		h.Generate(w, req(http.MethodPost, "/api/testgen", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("testgen missing fields expected 400")
		}
	}
	// sqlgen
	{
		h := NewSQLGenHandler(fakeSQL{})
		w := httptest.NewRecorder()
		h.GenerateQuery(w, req(http.MethodPost, "/api/sql/query", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("sqlgen missing fields expected 400")
		}
	}
	// cicd
	{
		h := NewCICDHandler(fakeCICD{})
		w := httptest.NewRecorder()
		h.GeneratePipeline(w, req(http.MethodPost, "/api/cicd/generate", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("cicd missing platform expected 400")
		}
	}
	// architect
	{
		h := NewArchitectHandler(fakeArchitect{})
		w := httptest.NewRecorder()
		h.SuggestRefactor(w, req(http.MethodPost, "/api/architect/refactor", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("architect missing path expected 400")
		}
	}
	// repo
	{
		h := NewRepoHandler(fakeRepo{})
		w := httptest.NewRecorder()
		h.Refactor(w, req(http.MethodPost, "/api/refactor", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("repo missing path expected 400")
		}
	}
	// docgen
	{
		h := NewDocGenHandler(nil)
		w := httptest.NewRecorder()
		h.GenerateFileDoc(w, req(http.MethodPost, "/api/docs/file", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("docgen missing path expected 400")
		}
	}
	// review
	{
		h := NewReviewHandler(nil)
		w := httptest.NewRecorder()
		h.ReviewDiff(w, req(http.MethodPost, "/api/review/diff", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("review missing diff expected 400")
		}
	}
	// security
	{
		h := NewSecurityHandler(nil)
		w := httptest.NewRecorder()
		h.ScanFile(w, req(http.MethodPost, "/api/security/scan/file", "{}"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("security missing path expected 400")
		}
	}
	// session
	{
		h := NewSessionHandler(nil, fakeRAGHandler{})
		w := httptest.NewRecorder()
		h.Create(w, req(http.MethodPost, "/api/sessions", "{}"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("session create unauthorized expected 401")
		}
	}
	// profile
	{
		h := NewProfileHandler(fakeProfileSvc{})
		w := httptest.NewRecorder()
		h.Get(w, req(http.MethodGet, "/api/profile", ""))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("profile get unauthorized expected 401")
		}
	}
	// patch preview
	{
		h := NewPatchHandler(".", fakePatchSvc{})
		w := httptest.NewRecorder()
		h.Preview(w, req(http.MethodGet, "/api/patch/preview", ""))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("patch preview missing response expected 400")
		}
	}
}

func TestHandlerSuccessSmoke(t *testing.T) {
	// health
	{
		h := NewHealthHandler(&config.Config{})
		w := httptest.NewRecorder()
		h.Liveness(w, req(http.MethodGet, "/health", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("health liveness expected 200")
		}
		w = httptest.NewRecorder()
		h.Readiness(w, req(http.MethodGet, "/health/readiness", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("health readiness expected 200")
		}
	}
	// metrics
	{
		h := NewMetricsHandler(fakeMetrics{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodGet, "/metrics", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("metrics expected 200")
		}
	}
	// indexer
	{
		h := NewIndexerHandler(fakeIndexer{})
		w := httptest.NewRecorder()
		h.Status(w, req(http.MethodGet, "/internal/index/status", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("indexer status expected 200")
		}
		w = httptest.NewRecorder()
		h.Reindex(w, req(http.MethodPost, "/internal/index/reindex", ""))
		if w.Code != http.StatusAccepted {
			t.Fatalf("indexer reindex expected 202")
		}
	}
	// repo analyze
	{
		h := NewRepoHandler(fakeRepo{})
		w := httptest.NewRecorder()
		h.Analyze(w, req(http.MethodGet, "/api/analyze-repo", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("repo analyze expected 200")
		}
	}
}
