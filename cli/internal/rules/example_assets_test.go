// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/estebanforge/pylons/cli/internal/report"
)

// The shipped examples are product files: a broken example teaches
// the wrong lesson. These tests drive the example plugin and the
// example ledger through the real engine, so wire or rule drift fails
// the build.

func isParityFinding(r string) bool {
	return strings.HasPrefix(r, "PYLON049") ||
		strings.HasPrefix(r, "PYLON05") ||
		r == "PYLON059"
}

func TestExampleLedgerLintsClean(t *testing.T) {
	findings := Lint(filepath.Join("..", "..", "..", "examples", "ledger"), LintOptions{Parity: true, HasCode: true})
	for _, f := range findings {
		if f.Grade == report.GradeForced {
			t.Errorf("example ledger carries FORCED %s: %s", f.RuleID, f.Message)
		}
		if isParityFinding(f.RuleID) {
			t.Errorf("example ledger carries parity finding %s: %s", f.RuleID, f.Message)
		}
	}
}

func TestExampleLedgerMutationBlocks(t *testing.T) {
	src := filepath.Join("..", "..", "..", "examples", "ledger")
	dst := filepath.Join(t.TempDir(), "ledger")
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	code := filepath.Join(dst, "svc", "shipments.go")
	body, err := os.ReadFile(code)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(body), "EventShip: ShipmentShipped,", "EventShip: ShipmentPicked,", 1)
	if mutated == string(body) {
		t.Fatal("mutation anchor missing; the example source changed shape")
	}
	if err := os.WriteFile(code, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	findings := Lint(dst, LintOptions{Parity: true, HasCode: true})
	var sawMismatch bool
	for _, f := range findings {
		if f.RuleID == "PYLON054" {
			sawMismatch = true
		}
	}
	if !sawMismatch {
		t.Errorf("mutated example ledger fired no PYLON054; findings: %d", len(findings))
	}
}

func TestExampleParityPluginExtracts(t *testing.T) {
	kb, root := t.TempDir(), t.TempDir()
	plugin, err := os.ReadFile(filepath.Join("..", "..", "..", "pylons", "extensions", "_examples", "pylon-ext-parity"))
	if err != nil {
		t.Fatalf("read shipped example plugin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(kb, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kb, "extensions", "pylon-ext-parity"), plugin, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(kb, "manifests", "pylons.yaml"), `version: 1
extensions: [parity]
parity:
  version: 1
  profiles:
    demo:
      use: external:parity
      extensions:
        - .demo
projects:
  svc:
    path: `+root+`
`)
	writeDomainMachine(t, kb, "repo://svc/flow.demo::demo")
	mustWrite(t, filepath.Join(root, "flow.demo"), `// pylon:machine demo
enum demo { Pending, Done }
Pending -- finish --> Done
`)
	if findings := lintParity(kb, cfgMust(t, kb), true); len(findings) != 0 {
		for _, f := range findings {
			t.Errorf("matching example machine fired %s: %s", f.RuleID, f.Message)
		}
	}

	// Drift: the code moves the triple the ledger does not declare.
	mutated := strings.Replace(
		"Pending -- finish --> Done",
		"finish", "cancel", 1,
	)
	mustWrite(t, filepath.Join(root, "flow.demo"), `// pylon:machine demo
enum demo { Pending, Done }
`+mutated+`
`)
	findings := lintParity(kb, cfgMust(t, kb), true)
	var sawMismatch bool
	for _, f := range findings {
		if f.RuleID == "PYLON054" {
			sawMismatch = true
		}
	}
	if !sawMismatch {
		t.Errorf("drifted example machine fired no PYLON054; findings: %d", len(findings))
	}
}

// writeDomainMachine declares the example machine exactly as the
// ledger's domain doc does.
func writeDomainMachine(t *testing.T, kb, symbol string) {
	t.Helper()
	fm := "---\ntitle: \"Flow\"\ntype: domain-entity\nstatus: accepted\ncreated: 2026-11-21\nupdated: 2026-11-21\nowners: [o]\nmachines:\n  - machine: demo\n    states: [Pending, Done]\n    initial: [Pending]\n    terminal: [Done]\n    transitions_symbol: \"" + symbol + "\"\n    transitions:\n      - {from: Pending, event: finish, to: Done}\ntags: [domain]\n---\n\n# Flow\n"
	mustWrite(t, filepath.Join(kb, "domain", "flow.md"), fm)
}

// copyTree copies a directory of example files, preserving modes, so
// tests can mutate a private copy.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, in)
		return err
	})
}
