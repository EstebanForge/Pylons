// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureFixture: a code repo with a KB inside it, one package doc and
// one plan declaring surfaces, and a second commit touching only code.
type captureFixture struct {
	repo string
	kb   string
}

func newCaptureFixture(t *testing.T) captureFixture {
	t.Helper()
	repo := t.TempDir()
	kb := repo
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	mustWrite(t, filepath.Join(repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x + 1\n}\n")
	mustWrite(t, filepath.Join(repo, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/app.md"), "---\ntitle: \"App\"\ntype: package\nstatus: active\ntier: core\nexposes:\n  - kind: symbol\n    id: repo://app/src/svc.go::Process\nsources:\n  - repo://app/src/svc.go\n---\n\n# App\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-api.md"), "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: route\n      id: repo://app/src/routes.go\n---\n\n# API\n")
	shell("git", "-C", repo, "add", "-fA", ".")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "docs: seed")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	return captureFixture{repo: repo, kb: kb}
}

func (fx captureFixture) touchCode(t *testing.T) {
	t.Helper()
	mustWrite(t, filepath.Join(fx.repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x\n}\n")
	mustWrite(t, filepath.Join(fx.repo, "src/routes.go"), "package src\n\n// route /process\nfunc handleProcess() {}\n")
	shell("git", "-C", fx.repo, "add", "-f", "src")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: extend api")
}

func (fx captureFixture) updateDocs(t *testing.T) {
	t.Helper()
	mustWrite(t, filepath.Join(fx.kb, "packages/app.md"), "---\ntitle: \"App\"\ntype: package\nstatus: active\ntier: core\nexposes:\n  - kind: symbol\n    id: repo://app/src/svc.go::Process\nsources:\n  - repo://app/src/svc.go\n---\n\n# App updated\n")
	shell("git", "-C", fx.repo, "add", "-f", "packages/app.md")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "docs: update app")
}

func TestCaptureFlagsTouchedStaleSurfaces(t *testing.T) {
	fx := newCaptureFixture(t)
	fx.touchCode(t)
	manifest, err := SessionCapture(fx.kb, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Surfaces) != 2 {
		t.Fatalf("svc.go (expose + source) and routes.go (plan contract) are stale; got %+v", manifest.Surfaces)
	}
	seen := map[string]bool{}
	for _, s := range manifest.Surfaces {
		seen[s.Path] = true
		if !strings.HasSuffix(s.Doc, ".md") {
			t.Fatalf("stale surface must anchor to a real KB doc, got %+v", s)
		}
	}
	if !seen["src/svc.go"] || !seen["src/routes.go"] {
		t.Fatalf("missing stale surfaces: %+v", manifest.Surfaces)
	}
}

func TestCaptureClearsSurfacesWhenDocsMove(t *testing.T) {
	fx := newCaptureFixture(t)
	fx.touchCode(t)
	fx.updateDocs(t)
	manifest, err := SessionCapture(fx.kb, "HEAD~2")
	if err != nil {
		t.Fatal(err)
	}
	// app.md moved in the commit set, so its surfaces are clear; the
	// plan contract's doc did not.
	var routesStale bool
	for _, s := range manifest.Surfaces {
		if s.Path == "src/routes.go" {
			routesStale = true
		}
		if s.Path == "src/svc.go" {
			t.Fatalf("a surface whose doc moved in the same commit set must clear, got %+v", manifest.Surfaces)
		}
	}
	if !routesStale {
		t.Fatalf("routes.go surface must stay stale, got %+v", manifest.Surfaces)
	}
}

func TestCaptureIgnoresUntouchedSurfaces(t *testing.T) {
	fx := newCaptureFixture(t)
	manifest, err := SessionCapture(fx.kb, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Surfaces) != 0 {
		t.Fatalf("an empty diff must yield an empty manifest, got %+v", manifest.Surfaces)
	}
}

func TestCaptureSkipsUnreadableProject(t *testing.T) {
	fx := newCaptureFixture(t)
	mustWrite(t, filepath.Join(fx.kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+fx.repo+"\n    ref: main\n  ghost:\n    path: /nonexistent/ghost\n    ref: main\n")
	mustWrite(t, filepath.Join(fx.kb, "plans/feature-ghost.md"), "---\ntitle: \"Ghost\"\ntype: plan\nstatus: accepted\ncomponents: [ghost]\nticket: [T-2]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://ghost/inner.go::Missing\n---\n\n# Ghost\n")
	fx.touchCode(t)
	manifest, err := SessionCapture(fx.kb, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	// The readable project still reports; the missing one is skipped
	// with a warning, never silently.
	for _, s := range manifest.Surfaces {
		if s.Alias == "ghost" {
			t.Fatalf("an unreadable project's surface must be skipped, got %+v", manifest.Surfaces)
		}
	}
	if len(manifest.Warnings) == 0 {
		t.Fatal("an unreadable project must produce a warning")
	}
}

func TestCaptureNestedKBSeesItsDocs(t *testing.T) {
	// A KB nested under a larger repository root: git prints
	// repo-root-relative paths, but the manifest compares KB-relative
	// docs.
	repo := t.TempDir()
	kb := filepath.Join(repo, "ledger")
	if err := os.MkdirAll(kb, 0o755); err != nil {
		t.Fatal(err)
	}
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	mustWrite(t, filepath.Join(repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x\n}\n")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "packages/app.md"), "---\ntitle: \"App\"\ntype: package\nstatus: active\ntier: core\nexposes:\n  - kind: symbol\n    id: repo://app/src/svc.go::Process\nsources:\n  - repo://app/src/svc.go\n---\n\n# App\n")
	shell("git", "-C", repo, "add", "-fA", ".")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "docs: seed")
	mustWrite(t, filepath.Join(repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x + 1\n}\n")
	shell("git", "-C", repo, "add", "-f", ".")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: change Process")
	manifest, err := SessionCapture(kb, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Surfaces) != 1 || manifest.Surfaces[0].Path != "src/svc.go" {
		t.Fatalf("a nested KB must judge its own docs, got %+v", manifest.Surfaces)
	}
}

func TestCaptureCountsUncommittedDocs(t *testing.T) {
	// At session close the doc edit is usually uncommitted; it still
	// clears the surface.
	fx := newCaptureFixture(t)
	fx.touchCode(t)
	mustWrite(t, filepath.Join(fx.kb, "packages/app.md"), "---\ntitle: \"App\"\ntype: package\nstatus: active\ntier: core\nexposes:\n  - kind: symbol\n    id: repo://app/src/svc.go::Process\nsources:\n  - repo://app/src/svc.go\n---\n\n# App updated\n")
	manifest, err := SessionCapture(fx.kb, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range manifest.Surfaces {
		if s.Path == "src/svc.go" {
			t.Fatalf("an uncommitted doc edit must clear the surface, got %+v", manifest.Surfaces)
		}
	}
}

func TestCaptureCountsUncommittedCode(t *testing.T) {
	fx := newCaptureFixture(t)
	mustWrite(t, filepath.Join(fx.repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x\n}\n")
	manifest, err := SessionCapture(fx.kb, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Surfaces) != 1 || manifest.Surfaces[0].Path != "src/svc.go" {
		t.Fatalf("uncommitted code edits must count, got %+v", manifest.Surfaces)
	}
}

func TestCaptureNestedDocPathIsReal(t *testing.T) {
	fx := newCaptureFixture(t)
	mustWrite(t, filepath.Join(fx.kb, "packages/sub/inner.md"), "---\ntitle: \"Inner\"\ntype: package\nstatus: active\ntier: core\nexposes:\n  - kind: symbol\n    id: repo://app/src/routes.go::handleProcess\n---\n\n# Inner\n")
	fx.touchCode(t)
	manifest, err := SessionCapture(fx.kb, "HEAD~1")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range manifest.Surfaces {
		if s.Doc == "packages/sub/inner.md" {
			found = true
		}
		if s.Doc == "packages/inner.md" {
			t.Fatalf("doc paths must be real KB-relative paths, got %+v", manifest.Surfaces)
		}
	}
	if !found {
		t.Fatalf("the nested doc must anchor the finding, got %+v", manifest.Surfaces)
	}
}

func TestCaptureRejectsEmptyRef(t *testing.T) {
	fx := newCaptureFixture(t)
	if _, err := SessionCapture(fx.kb, ""); err == nil {
		t.Fatal("an empty ref must be a usage error")
	}
}
