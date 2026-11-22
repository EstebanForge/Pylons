// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// releaseSurfaceFixture builds a code repo whose KB lives inside it, a
// tagged release, and a deployment dir pinning that release.
type releaseSurfaceFixture struct {
	repo    string
	kb      string
	deploy  string
	lockDir string
}

func newReleaseSurfaceFixture(t *testing.T, planDoc string) releaseSurfaceFixture {
	t.Helper()
	repo := t.TempDir()
	kb := repo
	deploy := filepath.Join(t.TempDir(), "deploy")
	if err := os.MkdirAll(deploy, 0o755); err != nil {
		t.Fatal(err)
	}
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	mustWrite(t, filepath.Join(repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x + 1\n}\n")
	mustWrite(t, filepath.Join(repo, "src/routes.go"), "package src\n\n// route /process\nfunc handleProcess() {}\n")
	mustWrite(t, filepath.Join(repo, "src/keep.go"), "package src\n\nfunc Keep() {}\n")
	shell("git", "-C", repo, "add", "-f", "src")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "tag", "0.1.0")
	shell("git", "-C", repo, "remote", "add", "origin", repo)
	shell("git", "-C", repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+repo+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-api.md"), planDoc)
	return releaseSurfaceFixture{repo: repo, kb: kb, deploy: deploy, lockDir: deploy}
}

func (fx releaseSurfaceFixture) pin(t *testing.T, version string, consumers map[string]string) {
	t.Helper()
	mustWrite(t, filepath.Join(fx.lockDir, "app.lock"), "app "+version+"\n")
	for name, body := range consumers {
		mustWrite(t, filepath.Join(fx.lockDir, name), body)
	}
}

func (fx releaseSurfaceFixture) breakSurface(t *testing.T) {
	t.Helper()
	mustWrite(t, filepath.Join(fx.repo, "src/svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x\n}\n")
	shell("git", "-C", fx.repo, "add", "-f", "src")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "refactor: change Process")
}

func findingsOf(fs []report.Finding, id string) []string {
	var msgs []string
	for _, f := range fs {
		if f.RuleID == id {
			msgs = append(msgs, f.Message)
		}
	}
	return msgs
}

func TestReleaseSurfaceRemovedBlocks(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n    - kind: symbol\n      id: repo://app/src/keep.go::Keep\n---\n\n# API\n")
	fx.pin(t, "0.1.0", map[string]string{"consumer.go": "out := Process(input)\n"})
	fx.breakSurface(t)
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_CHANGED") {
		t.Fatalf("consumed changed surface must block once, got %v", msgs)
	}
}

func TestReleaseSurfaceRemovedFileBlocks(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: route\n      id: repo://app/src/routes.go\n---\n\n# API\n")
	// A vendored copy of the route file counts as consumption.
	fx.pin(t, "0.1.0", map[string]string{"consumer/routes.go": "package consumer\n\n// vendored route /process\nfunc handleProcess() {}\n"})
	mustRemove(t, filepath.Join(fx.repo, "src/routes.go"))
	shell("git", "-C", fx.repo, "add", "-fA", ".")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "refactor: drop routes")
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_REMOVED") {
		t.Fatalf("consumed removed surface must block, got %v", msgs)
	}
}

func TestReleaseSurfaceUnconsumedStaysSilent(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "0.1.0", nil)
	fx.breakSurface(t)
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	if msgs := findingsOf(fs, "PYLON048"); len(msgs) != 0 {
		t.Fatalf("a surface no deployment consumes must stay silent, got %v", msgs)
	}
}

func TestReleaseSurfaceUnknownPinFailsClosed(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	mustWrite(t, filepath.Join(fx.lockDir, "web.lock"), "ghost 0.1.0\n")
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) == 0 || !strings.Contains(msgs[0], "RELEASE_PIN_UNKNOWN") {
		t.Fatalf("unregistered pin alias must fail closed, got %v", msgs)
	}
}

func TestReleaseSurfaceMissingTagFailsClosed(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "9.9.9", map[string]string{"consumer.go": "Process(1)\n"})
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) == 0 || !strings.Contains(msgs[0], "RELEASE_TAG_MISSING") {
		t.Fatalf("unresolvable pinned tag must fail closed, got %v", msgs)
	}
}

func TestReleaseSurfaceNotEvaluated(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) == 0 || !strings.Contains(msgs[0], "RELEASE_SURFACE_NOT_EVALUATED") {
		t.Fatalf("no pins at all must fail open loudly, got %v", msgs)
	}
}

func TestReleaseSurfaceBaseOverridesPins(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "9.9.9", map[string]string{"consumer.go": "Process(1)\n"})
	fx.breakSurface(t)
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: "origin/main"})
	if err != nil {
		t.Fatal(err)
	}
	// The pinned tag is unresolvable, but --base replaces pinning, so the
	// diff runs and the changed surface blocks.
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_CHANGED") {
		t.Fatalf("--base must override pinning, got %v", msgs)
	}
}

func mustRemove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseSurfaceConsumptionIsPerSurface(t *testing.T) {
	// Surface X changes while only surface Y is consumed: the gate
	// must stay silent, and must fire only when Y itself changes.
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n    - kind: symbol\n      id: repo://app/src/keep.go::Keep\n---\n\n# API\n")
	fx.pin(t, "0.1.0", map[string]string{"consumer.go": "fmt.Sprint(Keep())\n"})
	fx.breakSurface(t)
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir})
	if err != nil {
		t.Fatal(err)
	}
	if msgs := findingsOf(fs, "PYLON048"); len(msgs) != 0 {
		t.Fatalf("a consumed sibling must not judge an unconsumed surface, got %v", msgs)
	}
}

func TestReleaseSurfaceSubdirProject(t *testing.T) {
	// A project rooted at a subdirectory of its repo: blob paths are
	// project-relative, and rev-parse must resolve them from there.
	repo := t.TempDir()
	proj := filepath.Join(repo, "sub")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	shell("git", "-C", repo, "init", "-q", "-b", "main")
	mustWrite(t, filepath.Join(proj, "svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x + 1\n}\n")
	shell("git", "-C", repo, "add", "-f", ".")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: base")
	shell("git", "-C", repo, "tag", "0.1.0")
	kb := t.TempDir()
	mustWrite(t, filepath.Join(kb, "manifests/pylons.yaml"), "version: 1\nprojects:\n  app:\n    path: "+proj+"\n    ref: main\n")
	mustWrite(t, filepath.Join(kb, "plans/feature-api.md"), "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/svc.go::Process\n---\n\n# API\n")
	deploy := filepath.Join(t.TempDir(), "deploy")
	if err := os.MkdirAll(deploy, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(deploy, "app.lock"), "app 0.1.0\n")
	mustWrite(t, filepath.Join(deploy, "consumer.go"), "Process(1)\n")
	mustWrite(t, filepath.Join(proj, "svc.go"), "package src\n\nfunc Process(x int) int {\n\treturn x\n}\n")
	shell("git", "-C", repo, "add", "-f", ".")
	shell("git", "-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "refactor: change Process")
	fs, err := ReleaseSurface(kb, ReleaseSurfaceOptions{PinnedDir: deploy})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_CHANGED") {
		t.Fatalf("a subdir project must be judged, got %v", msgs)
	}
}

func TestReleaseSurfaceLockfileCRLF(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	mustWrite(t, filepath.Join(fx.lockDir, "app.lock"), "app 0.1.0\r\n")
	mustWrite(t, filepath.Join(fx.lockDir, "consumer.go"), "Process(1)\n")
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	// The pinned tag exists and the surface is unchanged: silence pins
	// the CRLF parse.
	if len(msgs) != 0 {
		t.Fatalf("a CRLF lockfile must parse cleanly, got %v", msgs)
	}
}

func TestReleaseSurfaceVendoredSymbolCopy(t *testing.T) {
	// A vendored copy counts for symbol surfaces too: the file carrying
	// the symbol is present under the deployment tree.
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "0.1.0", map[string]string{"vendor/src/svc.go": "package src\n\nfunc Process(x int) int {\n\treturn x + 1\n}\n"})
	fx.breakSurface(t)
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_CHANGED") {
		t.Fatalf("a vendored symbol copy must count as consumption, got %v", msgs)
	}
}

func TestReleaseSurfaceDashVersionFailsClosed(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/svc.go::Process\n---\n\n# API\n")
	fx.pin(t, "--exec=evil", map[string]string{"consumer.go": "Process(1)\n"})
	fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir})
	if err != nil {
		t.Fatal(err)
	}
	msgs := findingsOf(fs, "PYLON048")
	if len(msgs) == 0 || !strings.Contains(msgs[0], "RELEASE_TAG_MISSING") {
		t.Fatalf("a dash-leading version must fail closed without reaching git as an option, got %v", msgs)
	}
}

func TestReleaseSurfaceTwoSymbolsOneFileDeterministic(t *testing.T) {
	fx := newReleaseSurfaceFixture(t, "---\ntitle: \"API\"\ntype: plan\nstatus: accepted\ncomponents: [app]\nticket: [T-1]\ncontract:\n  stable:\n    - kind: symbol\n      id: repo://app/src/api.go::Alpha\n    - kind: symbol\n      id: repo://app/src/api.go::Beta\n---\n\n# API\n")
	mustWrite(t, filepath.Join(fx.repo, "src/api.go"), "package src\n\nfunc Alpha(x int) int {\n\treturn x + 1\n}\n\nfunc Beta(x int) int {\n\treturn x + 2\n}\n")
	shell("git", "-C", fx.repo, "add", "-f", "src")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "feat: api")
	shell("git", "-C", fx.repo, "tag", "0.2.0")
	fx.pin(t, "0.2.0", map[string]string{"consumer.go": "out := Alpha(input)\n"})
	// Change the file at HEAD so the declared surfaces drift; only
	// Alpha is consumed, but both live in the same file.
	mustWrite(t, filepath.Join(fx.repo, "src/api.go"), "package src\n\nfunc Alpha(x int) int {\n\treturn x\n}\n\nfunc Beta(x int) int {\n\treturn x + 2\n}\n")
	shell("git", "-C", fx.repo, "add", "-f", "src")
	shell("git", "-C", fx.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "refactor: change Alpha")
	for i := 0; i < 10; i++ {
		fs, err := ReleaseSurface(fx.kb, ReleaseSurfaceOptions{PinnedDir: fx.lockDir, Base: ""})
		if err != nil {
			t.Fatal(err)
		}
		msgs := findingsOf(fs, "PYLON048")
		if len(msgs) != 1 || !strings.Contains(msgs[0], "RELEASE_SURFACE_CHANGED") {
			t.Fatalf("run %d: a file whose consumed symbol changed must block deterministically, got %v", i, msgs)
		}
	}
}
