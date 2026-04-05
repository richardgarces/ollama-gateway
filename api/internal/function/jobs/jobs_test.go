package service

import (
	"context"
	"errors"
	"testing"
	"time"

	archdomain "ollama-gateway/internal/function/core/domain"
	securitydomain "ollama-gateway/internal/function/security/domain"
)

type fakeSecurityScanner struct{}

func (f *fakeSecurityScanner) ScanRepo() (securitydomain.SecurityReport, error) {
	return securitydomain.SecurityReport{ScannedFiles: 3, TotalFindings: 2}, nil
}

type fakeDocGen struct{}

func (f *fakeDocGen) GenerateREADME(repoRoot string) (string, error) {
	return "# README", nil
}

func (f *fakeDocGen) WriteWithBackup(path string, content string) (string, error) {
	return path + ".bak", nil
}

type fakeArchitect struct{}

func (f *fakeArchitect) AnalyzeProject() (archdomain.ArchReport, error) {
	return archdomain.ArchReport{Score1To10: 7}, nil
}

type blockingIndexer struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingIndexer) IndexRepo() error {
	close(b.started)
	<-b.release
	return nil
}

func TestJobsServiceCreateAndGetResult(t *testing.T) {
	svc := NewService(Dependencies{
		Workers:  1,
		Security: &fakeSecurityScanner{},
	})
	t.Cleanup(func() {
		_ = svc.Shutdown(context.Background())
	})

	job, err := svc.Create(CreateInput{Type: JobTypeSecurityScanRepo, RequestedBy: "dev1"})
	if err != nil {
		t.Fatalf("create error: %v", err)
	}
	if job.Status != JobStatusQueued {
		t.Fatalf("expected queued status, got %s", job.Status)
	}

	waitForStatus(t, svc, job.ID, JobStatusSucceeded)

	result, status, err := svc.GetResult(job.ID)
	if err != nil {
		t.Fatalf("get result error: %v", err)
	}
	if status != JobStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", status)
	}
	report, ok := result.(securitydomain.SecurityReport)
	if !ok {
		t.Fatalf("expected SecurityReport result, got %T", result)
	}
	if report.TotalFindings != 2 {
		t.Fatalf("expected findings 2, got %d", report.TotalFindings)
	}
}

func TestJobsServiceCancelQueuedJob(t *testing.T) {
	idx := &blockingIndexer{started: make(chan struct{}), release: make(chan struct{})}
	svc := NewService(Dependencies{
		Workers:  1,
		QueueSize: 8,
		Indexer:  idx,
		Security: &fakeSecurityScanner{},
	})
	t.Cleanup(func() {
		close(idx.release)
		_ = svc.Shutdown(context.Background())
	})

	first, err := svc.Create(CreateInput{Type: JobTypeIndexerReindex})
	if err != nil {
		t.Fatalf("create first job: %v", err)
	}

	select {
	case <-idx.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("indexer job did not start")
	}

	second, err := svc.Create(CreateInput{Type: JobTypeSecurityScanRepo})
	if err != nil {
		t.Fatalf("create second job: %v", err)
	}

	job, err := svc.Cancel(second.ID)
	if err != nil {
		t.Fatalf("cancel error: %v", err)
	}
	if job.Status != JobStatusCanceled {
		t.Fatalf("expected canceled status, got %s", job.Status)
	}

	_, status, err := svc.GetResult(second.ID)
	if !errors.Is(err, ErrJobCanceled) {
		t.Fatalf("expected ErrJobCanceled, got %v", err)
	}
	if status != JobStatusCanceled {
		t.Fatalf("expected canceled status in result, got %s", status)
	}

	close(idx.release)
	waitForStatus(t, svc, first.ID, JobStatusSucceeded)
}

func TestJobsServiceInvalidType(t *testing.T) {
	svc := NewService(Dependencies{Workers: 1})
	t.Cleanup(func() {
		_ = svc.Shutdown(context.Background())
	})

	if _, err := svc.Create(CreateInput{Type: JobType("unknown")}); err == nil {
		t.Fatalf("expected invalid type error")
	}
}

func waitForStatus(t *testing.T, svc *Service, id string, want JobStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err := svc.Get(id)
		if err == nil && job.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	job, err := svc.Get(id)
	if err != nil {
		t.Fatalf("job no disponible: %v", err)
	}
	t.Fatalf("timeout esperando status %s; último status=%s", want, job.Status)
}
