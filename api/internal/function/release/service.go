package release

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type ReleaseNotes struct {
	Features        []string
	Fixes           []string
	BreakingChanges []string
	Security        []string
	Raw             string
}

type Service struct {
	repoRoot string
}

func NewService(repoRoot string) *Service {
	return &Service{repoRoot: strings.TrimSpace(repoRoot)}
}

func (s *Service) BuildReleaseNotes(fromRef, toRef string) (*ReleaseNotes, error) {
	return BuildReleaseNotes(fromRef, toRef)
}

func (s *Service) WriteChangelog(notes *ReleaseNotes) error {
	return WriteChangelog(notes, WriteOptions{RepoRoot: s.repoRoot})
}

func BuildReleaseNotes(fromRef, toRef string) (*ReleaseNotes, error) {
	from := strings.TrimSpace(fromRef)
	to := strings.TrimSpace(toRef)
	if from == "" || to == "" {
		return nil, fmt.Errorf("fromRef y toRef son requeridos")
	}

	cmd := exec.Command("git", "log", fmt.Sprintf("%s..%s", from, to), "--pretty=format:%H%x1f%s%x1f%b%x1e")
	if wd := strings.TrimSpace(os.Getenv("REPO_ROOT")); wd != "" {
		cmd.Dir = wd
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error ejecutando git log: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	notes := &ReleaseNotes{Raw: string(output)}
	entries := strings.Split(string(output), "\x1e")
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, "\x1f", 3)
		if len(parts) < 2 {
			continue
		}

		subject := strings.TrimSpace(parts[1])
		body := ""
		if len(parts) == 3 {
			body = strings.TrimSpace(parts[2])
		}
		categorizeCommit(notes, subject, body)
	}

	return notes, nil
}

var conventionalCommitRE = regexp.MustCompile(`^([a-zA-Z]+)(\([^)]+\))?(!)?:\s*(.+)$`)

func categorizeCommit(notes *ReleaseNotes, subject, body string) {
	if notes == nil {
		return
	}

	commitType, summary, breakingByType := parseConventionalSubject(subject)
	if summary == "" {
		summary = strings.TrimSpace(subject)
	}
	if summary == "" {
		return
	}

	summaryLower := strings.ToLower(summary)
	bodyLower := strings.ToLower(body)

	isBreaking := breakingByType || strings.Contains(bodyLower, "breaking change:") || strings.Contains(bodyLower, "breaking-change:")
	isSecurity := commitType == "security" || commitType == "sec" || strings.Contains(summaryLower, "security") || strings.Contains(summaryLower, "cve-")

	switch commitType {
	case "feat":
		notes.Features = append(notes.Features, summary)
	case "fix":
		notes.Fixes = append(notes.Fixes, summary)
	}

	if isBreaking {
		notes.BreakingChanges = append(notes.BreakingChanges, summary)
	}
	if isSecurity {
		notes.Security = append(notes.Security, summary)
	}
}

func parseConventionalSubject(subject string) (commitType string, summary string, breaking bool) {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "", "", false
	}

	matches := conventionalCommitRE.FindStringSubmatch(trimmed)
	if len(matches) == 0 {
		return "", trimmed, false
	}

	return strings.ToLower(matches[1]), strings.TrimSpace(matches[4]), matches[3] == "!"
}
