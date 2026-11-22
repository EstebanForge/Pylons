// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// recallFixture builds a fixed sample ledger whose documents carry
// deliberately overlapping vocabulary so ranking quality is observable.
func recallFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeDoc := func(rel, front, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("---\n"+front+"---\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeDoc("quirks/webhook-replays.md",
		"title: Webhook replay guard\ntype: quirk\nstatus: active\ncreated: 2026-01-02\nupdated: 2026-06-01\npackage: core\ncomponents: [webhook-delivery]\ntags: [webhook, replay]\n",
		"Duplicate webhook deliveries replayed old batches. The receiver now records an idempotency key per delivery so a replayed webhook is dropped.\n")
	writeDoc("plans/feature-tracking.md",
		"title: Shipment tracking\ntype: plan\nstatus: accepted\ncreated: 2026-02-01\nupdated: 2026-06-10\nticket: [EX-1]\ncomponents: [tracking]\n",
		"The tracking plan follows a shipment from pickup to delivery and exposes milestone events.\n")
	writeDoc("domain/shipments.md",
		"title: Shipment\ntype: domain-entity\nstatus: active\ncreated: 2026-01-03\nupdated: 2026-05-20\n",
		"A shipment lifecycle moves through pickup, transit, and delivery states.\n")
	writeDoc("conventions/naming.md",
		"title: Naming convention\ntype: convention\nstatus: accepted\ncreated: 2026-01-04\nupdated: 2026-04-02\nlevel: MUST\n",
		"Exported names follow the naming convention: plain words, no abbreviations.\n")
	writeDoc("packages/core.md",
		"title: Core package\ntype: package\nstatus: active\ncreated: 2026-01-05\nupdated: 2026-06-12\npackage: core\n",
		"The core package owns the webhook delivery pipeline and the shared event bus.\n")
	writeDoc("decisions/0001-grpc-transport.md",
		"title: gRPC transport\ntype: decision\nstatus: accepted\ncreated: 2026-01-06\nupdated: 2026-03-15\n",
		"Internal transport uses gRPC. The decision records why gRPC beat a REST gateway.\n")
	writeDoc("investigations/2026-01-05-flaky-tests.md",
		"title: Flaky integration tests\ntype: investigation\nstatus: resolved\ncreated: 2026-01-05\nupdated: 2026-02-01\nseverity: high\n",
		"The flaky integration tests raced on fixture setup. Serializing the race fixed the flakiness.\n")
	writeDoc("domain/billing.md",
		"title: Billing\ntype: domain-entity\nstatus: active\ncreated: 2026-01-07\nupdated: 2026-05-30\n",
		"An invoice closes at the end of a billing cycle. Billing terms live with the cycle.\n")
	writeDoc("quirks/portal-logins.md",
		"title: Portal login failures\ntype: quirk\nstatus: active\ncreated: 2026-01-08\nupdated: 2026-06-02\ncomponents: [portal]\n",
		"Portal logins dropped the session cookie because SameSite rejected the redirect.\n")
	writeDoc("plans/archive/feature-old-import.md",
		"title: Old import plan\ntype: plan\nstatus: abandoned\ncreated: 2025-01-01\nupdated: 2025-02-01\n",
		"An abandoned import plan kept only for reference.\n")

	return root
}

func TestRetrievalRecall(t *testing.T) {
	root := recallFixture(t)
	docs := scanDocs(root, false)
	if len(docs) < 9 {
		t.Fatalf("fixture docs missing: got %d", len(docs))
	}

	cases := []struct {
		name   string
		query  string
		target string
		k      int
	}{
		{"exact quirk vocabulary", "webhook replay duplicate delivery", "quirks/webhook-replays.md", 1},
		{"quirk by mechanism", "idempotency key webhook", "quirks/webhook-replays.md", 1},
		{"decision by choice", "grpc transport decision", "decisions/0001-grpc-transport.md", 1},
		{"investigation by symptom", "flaky test race", "investigations/2026-01-05-flaky-tests.md", 1},
		{"domain by terms", "invoice billing cycle", "domain/billing.md", 1},
		{"convention by topic", "naming convention exports", "conventions/naming.md", 1},
		{"entity by lifecycle", "shipment lifecycle states", "domain/shipments.md", 1},
		{"second quirk", "session cookie login", "quirks/portal-logins.md", 1},
		{"single shared term", "webhook", "quirks/webhook-replays.md", 3},
		{"plan by shared term", "tracking", "plans/feature-tracking.md", 3},
		{"package by role", "package core webhook delivery", "packages/core.md", 3},
		{"component only in frontmatter", "portal", "quirks/portal-logins.md", 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterDocs(docs, "", "", "", "", "", tc.query, 10, nil)
			rank := -1
			for i, d := range got {
				if d.Path == tc.target {
					rank = i + 1
					break
				}
			}
			if rank == -1 {
				t.Fatalf("query %q: target %q not in results (%d docs)", tc.query, tc.target, len(got))
			}
			if rank > tc.k {
				t.Fatalf("query %q: target %q ranked %d, want top %d", tc.query, tc.target, rank, tc.k)
			}
		})
	}
}
