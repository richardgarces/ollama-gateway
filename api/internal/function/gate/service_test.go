package gate

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ollama-gateway/internal/function/core/domain"
)

type fakeScanner struct {
	report domain.SecurityReport
	err    error
}

func (f fakeScanner) ScanRepo() (domain.SecurityReport, error) {
	if f.err != nil {
		return domain.SecurityReport{}, f.err
	}
	return f.report, nil
}

type fakeRunner struct {
	commands []string
	outputs  map[string]string
	errors   map[string]error
}

func (f *fakeRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	_ = ctx
	key := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, key)
	if err, ok := f.errors[key]; ok {
		return f.outputs[key], err
	}
	return f.outputs[key], nil
}

func TestCheckDeployGateWith(t *testing.T) {
	t.Run("strict blocks on findings and low coverage", func(t *testing.T) {
		runner := &fakeRunner{
			outputs: map[string]string{
				"go test ./...":        "FAIL\n",
				"go test -cover ./...": "ok pkg coverage: 52.3% of statements\n",
			},
			errors: map[string]error{
				"go test ./...": fmt.Errorf("tests failed"),
			},
		}

		svc := NewService("/tmp/repo", fakeScanner{report: domain.SecurityReport{HighOrCritical: 3, TotalFindings: 8, ScannedFiles: 20}}, nil)
		svc.SetRunner(runner)

		result, err := svc.CheckDeployGateWith("strict", "prod")
		if err != nil {
			t.Fatalf("CheckDeployGateWith() error = %v", err)
		}
		if result.Allow {
			t.Fatalf("expected gate deny for strict profile")
		}
		if result.Profile != "strict" || result.Environment != "prod" {
			t.Fatalf("unexpected profile/environment: %+v", result)
		}
		if result.Coverage.Percent != 52.3 {
			t.Fatalf("unexpected coverage: %.2f", result.Coverage.Percent)
		}
		if result.Tests.Passed {
			t.Fatalf("expected failed tests")
		}
		if len(result.Reasons) < 2 {
			t.Fatalf("expected blocking reasons")
		}
	})

	t.Run("relaxed allows healthy state", func(t *testing.T) {
		runner := &fakeRunner{
			outputs: map[string]string{
				"go test ./...":        "ok\n",
				"go test -cover ./...": "ok pkg coverage: 73.0% of statements\n",
			},
			errors: map[string]error{},
		}
		svc := NewService("/tmp/repo", fakeScanner{report: domain.SecurityReport{HighOrCritical: 1, TotalFindings: 3, ScannedFiles: 11}}, nil)
		svc.SetRunner(runner)

		result, err := svc.CheckDeployGateWith("relaxed", "staging")
		if err != nil {
			t.Fatalf("CheckDeployGateWith() error = %v", err)
		}
		if !result.Allow {
			t.Fatalf("expected gate allow in relaxed profile")
		}
		if !result.Coverage.MeetsMin {
			t.Fatalf("expected coverage meets minimum")
		}
		if !result.Tests.Passed {
			t.Fatalf("expected tests passed")
		}
	})
}

func TestCheckDeployGateRequiresScanner(t *testing.T) {
	_, err := (&Service{}).CheckDeployGate()
	if err == nil {
		t.Fatalf("expected error for nil scanner")
	}
}
