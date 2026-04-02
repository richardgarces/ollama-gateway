package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ollama-gateway/internal/services"
)

type fakeDocGen struct{}

func (fakeDocGen) GenerateDocForFile(path string) (string, error) { return "package x", nil }
func (fakeDocGen) GenerateREADME(repoRoot string) (string, error) { return "# README", nil }
func (fakeDocGen) WriteWithBackup(path string, content string) (string, error) {
	return path + ".bak", nil
}

func TestHandlersSuccessPaths(t *testing.T) {
	// agent
	{
		h := NewAgentHandler(fakeAgent{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/agent", `{"input":"hola"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("agent expected 200")
		}
	}
	// generate non-stream
	{
		h := NewGenerateHandler(fakeRAGHandler{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/generate", `{"prompt":"hola","stream":false}`))
		if w.Code != http.StatusOK {
			t.Fatalf("generate expected 200")
		}
	}
	// chat non-stream
	{
		h := NewChatHandler(fakeRAGHandler{})
		w := httptest.NewRecorder()
		h.Handle(w, req(http.MethodPost, "/api/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}]}`))
		if w.Code != http.StatusOK {
			t.Fatalf("chat expected 200")
		}
	}
	// debug
	{
		h := NewDebugHandler(fakeDebugService{})
		w := httptest.NewRecorder()
		h.AnalyzeError(w, req(http.MethodPost, "/api/debug/error", `{"stack_trace":"panic"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("debug expected 200")
		}
	}
	// translator
	{
		h := NewTranslatorHandler(fakeTranslator{})
		w := httptest.NewRecorder()
		h.Translate(w, req(http.MethodPost, "/api/translate", `{"code":"print(1)","from":"python","to":"go"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("translate expected 200")
		}
		w = httptest.NewRecorder()
		h.TranslateFile(w, req(http.MethodPost, "/api/translate/file", `{"path":"x.go","to":"typescript"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("translate file expected 200")
		}
	}
	// testgen
	{
		h := NewTestGenHandler(fakeTestGen{})
		w := httptest.NewRecorder()
		h.Generate(w, req(http.MethodPost, "/api/testgen", `{"code":"func x(){}","lang":"go"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("testgen expected 200")
		}
		w = httptest.NewRecorder()
		h.GenerateForFile(w, req(http.MethodPost, "/api/testgen/file", `{"path":"x.go","apply":true}`))
		if w.Code != http.StatusOK {
			t.Fatalf("testgen file expected 200")
		}
	}
	// sqlgen
	{
		h := NewSQLGenHandler(fakeSQL{})
		w := httptest.NewRecorder()
		h.GenerateQuery(w, req(http.MethodPost, "/api/sql/query", `{"description":"listar","dialect":"postgres"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("sql query expected 200")
		}
		w = httptest.NewRecorder()
		h.GenerateMigration(w, req(http.MethodPost, "/api/sql/migration", `{"description":"crear","dialect":"postgres"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("sql migration expected 200")
		}
		w = httptest.NewRecorder()
		h.ExplainQuery(w, req(http.MethodPost, "/api/sql/explain", `{"sql":"select 1"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("sql explain expected 200")
		}
	}
	// cicd
	{
		h := NewCICDHandler(fakeCICD{})
		w := httptest.NewRecorder()
		h.GeneratePipeline(w, req(http.MethodPost, "/api/cicd/generate", `{"platform":"github-actions","apply":true}`))
		if w.Code != http.StatusOK {
			t.Fatalf("cicd generate expected 200")
		}
		w = httptest.NewRecorder()
		h.OptimizePipeline(w, req(http.MethodPost, "/api/cicd/optimize", `{"platform":"gitlab-ci","pipeline":"stages: [test]"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("cicd optimize expected 200")
		}
	}
	// architect
	{
		h := NewArchitectHandler(fakeArchitect{})
		w := httptest.NewRecorder()
		h.AnalyzeProject(w, req(http.MethodGet, "/api/architect/analyze", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("architect analyze expected 200")
		}
		w = httptest.NewRecorder()
		h.SuggestRefactor(w, req(http.MethodPost, "/api/architect/refactor", `{"path":"a.go"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("architect suggest expected 200")
		}
	}
	// repo
	{
		h := NewRepoHandler(fakeRepo{})
		w := httptest.NewRecorder()
		h.Refactor(w, req(http.MethodPost, "/api/refactor", `{"path":"./README.md","prompt":"x"}`))
		if w.Code != http.StatusOK {
			t.Fatalf("repo refactor expected 200")
		}
	}
	// docgen
	{
		h := NewDocGenHandler(fakeDocGen{})
		w := httptest.NewRecorder()
		h.GenerateFileDoc(w, req(http.MethodPost, "/api/docs/file", `{"path":"a.go","apply":true}`))
		if w.Code != http.StatusOK {
			t.Fatalf("docgen file expected 200")
		}
		w = httptest.NewRecorder()
		h.GenerateREADME(w, req(http.MethodPost, "/api/docs/readme", `{"apply":false}`))
		if w.Code != http.StatusOK {
			t.Fatalf("docgen readme expected 200")
		}
	}
	// patch
	{
		h := NewPatchHandler(".", fakePatchSvc{})
		w := httptest.NewRecorder()
		h.Apply(w, req(http.MethodPost, "/api/patch", `{"response":"abc","apply":true}`))
		if w.Code != http.StatusOK {
			t.Fatalf("patch apply expected 200")
		}
		w = httptest.NewRecorder()
		h.Preview(w, req(http.MethodGet, "/api/patch/preview?response=abc&limit=2", ""))
		if w.Code != http.StatusOK {
			t.Fatalf("patch preview expected 200")
		}
	}
}

func TestReviewAndSecurityHandlerWithRealServices(t *testing.T) {
	repoRoot := t.TempDir()
	f := filepath.Join(repoRoot, "a.go")
	if err := os.WriteFile(f, []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	rag := fakeRAG{response: `[]`}
	reviewSvc := services.NewReviewService(rag, repoRoot, nil)
	securitySvc := services.NewSecurityService(rag, repoRoot, nil)

	reviewH := NewReviewHandler(reviewSvc)
	w := httptest.NewRecorder()
	reviewH.ReviewFile(w, req(http.MethodPost, "/api/review/file", `{"path":"a.go"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("review file expected 200, got %d", w.Code)
	}

	securityH := NewSecurityHandler(securitySvc)
	w = httptest.NewRecorder()
	securityH.ScanFile(w, req(http.MethodPost, "/api/security/scan/file", `{"path":"a.go"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("security file expected 200, got %d", w.Code)
	}
}

func TestOpenAIAndSearchBasicPaths(t *testing.T) {
	ollama := fakeOllamaClient{}
	search := NewSearchHandler(ollama, fakeVectorStore{}, []string{"."})
	w := httptest.NewRecorder()
	search.Handle(w, req(http.MethodPost, "/api/search", `{"query":"x","top":2}`))
	if w.Code != http.StatusOK {
		t.Fatalf("search expected 200")
	}

	oai := NewOpenAIHandler(ollama, fakeRAGHandler{}, nil, nil)
	w = httptest.NewRecorder()
	oai.Embeddings(w, req(http.MethodPost, "/openai/v1/embeddings", `{"input":"hello"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("embeddings expected 200")
	}
	w = httptest.NewRecorder()
	oai.Completions(w, req(http.MethodPost, "/openai/v1/completions", `{"prompt":"hello","stream":false}`))
	if w.Code != http.StatusOK {
		t.Fatalf("completions expected 200")
	}
}

type fakeOllamaClient struct{}

func (fakeOllamaClient) Generate(model, prompt string) (string, error) { return "ok", nil }
func (fakeOllamaClient) StreamGenerate(model, prompt string, onChunk func(string) error) error {
	return onChunk("ok")
}
func (fakeOllamaClient) GetEmbedding(model, text string) ([]float64, error) {
	return []float64{0.1, 0.2}, nil
}

type fakeVectorStore struct{}

func (fakeVectorStore) UpsertPoint(collection, id string, vector []float64, payload map[string]interface{}) error {
	return nil
}
func (fakeVectorStore) Search(collection string, vector []float64, limit int) (map[string]interface{}, error) {
	return map[string]interface{}{"result": []interface{}{}}, nil
}

// local fakeRAG for handlers package

type fakeRAG struct{ response string }

func (f fakeRAG) GenerateWithContext(prompt string) (string, error) { return f.response, nil }
func (f fakeRAG) StreamGenerateWithContext(prompt string, onChunk func(string) error) error {
	return onChunk(f.response)
}
