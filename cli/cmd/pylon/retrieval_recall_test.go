// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// recallFixture builds a fixed sample ledger whose documents carry
// deliberately overlapping vocabulary so ranking quality is observable.
// Update dates are generated relative to today so the recency buckets in
// fusionBoost stay stable as the calendar moves.
func recallFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	date := func(daysAgo int) string {
		return time.Now().AddDate(0, 0, -daysAgo).Format("2006-01-02")
	}
	writeDoc := func(rel, docType, status, updated, front, body string, title ...string) {
		t.Helper()
		name := titleOf(rel)
		if len(title) == 1 {
			name = title[0]
		}
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		f := "---\ntitle: " + name + "\ntype: " + docType + "\nstatus: " + status + "\ncreated: " + date(400) + "\nupdated: " + updated + "\n" + front + "---\n" + body
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeDoc("quirks/webhook-replays.md", "quirk", "active", date(30),
		"package: core\ncomponents: [webhook-delivery]\ntags: [webhook, replay]\n",
		"Duplicate webhook deliveries replayed old batches. The receiver now records an idempotency key per delivery so a replayed webhook is dropped.\n")
	writeDoc("plans/feature-tracking.md", "plan", "accepted", date(200),
		"ticket: [EX-1]\ncomponents: [tracking]\n",
		"The tracking plan follows a shipment from pickup to delivery and exposes milestone events.\n")
	writeDoc("domain/shipments.md", "domain-entity", "active", date(150), "",
		"A shipment lifecycle moves through pickup, transit, and delivery states.\n")
	writeDoc("conventions/naming.md", "convention", "accepted", date(250), "level: MUST\n",
		"Exported names follow the naming convention: plain words, no abbreviations.\n")
	writeDoc("packages/core.md", "package", "active", date(10), "package: core\n",
		"The core package owns the webhook delivery pipeline and the shared event bus.\n")
	writeDoc("decisions/0001-grpc-transport.md", "decision", "accepted", date(300), "",
		"Internal transport uses gRPC. The decision records why gRPC beat a REST gateway.\n")
	writeDoc("investigations/2026-01-05-flaky-tests.md", "investigation", "resolved", date(350), "severity: high\n",
		"The flaky integration tests raced on fixture setup. Serializing the race fixed the flakiness.\n")
	writeDoc("domain/billing.md", "domain-entity", "active", date(220), "",
		"An invoice closes at the end of a billing cycle. Billing terms live with the cycle.\n")
	writeDoc("quirks/portal-logins.md", "quirk", "active", date(20),
		"components: [portal]\n",
		"Portal logins dropped the session cookie because SameSite rejected the redirect.\n")
	writeDoc("plans/archive/feature-old-import.md", "plan", "abandoned", date(500), "",
		"An abandoned import plan kept only for reference.\n")

	// Fusion-pinning pairs: equal term frequency and length make BM25 tie,
	// so only the fusion signals decide the order.
	writeDoc("quirks/alpha-webhook.md", "quirk", "active", date(30), "",
		"The webhook contract holds.\n")
	writeDoc("decisions/0002-webhook-old.md", "decision", "abandoned", date(400), "",
		"The webhook ledger closed.\n", "old webhook")
	writeDoc("quirks/invoice-notes.md", "quirk", "active", date(30), "",
		"Invoice terms ship quarterly.\n")
	writeDoc("decisions/0003-invoice.md", "decision", "active", date(400), "",
		"The invoice record survives.\n")
	writeDoc("quirks/ratchet-drift.md", "quirk", "active", date(40), "",
		"PYLON042 guards the advisory count ratchet.\n")
	writeDoc("decisions/0004-ratchet.md", "decision", "abandoned", date(400), "",
		"The ratchet budget used PYLON042 once.\n")
	writeDoc("domain/delivery-page.md", "domain-entity", "abandoned", date(400), "package: portal\n",
		"The delivery page shows status.\n")

	return root
}

func titleOf(rel string) string {
	base := filepath.Base(rel)
	if ext := filepath.Ext(base); ext != "" {
		base = base[:len(base)-len(ext)]
	}
	return base
}

func TestRetrievalRecall(t *testing.T) {
	root := recallFixture(t)
	docs := scanDocs(root, false)
	if len(docs) < 15 {
		t.Fatalf("fixture docs missing: got %d", len(docs))
	}

	cases := []struct {
		name   string
		query  string
		target string
		k      int
		projs  []string
	}{
		{"exact quirk vocabulary", "webhook replay duplicate delivery", "quirks/webhook-replays.md", 1, nil},
		{"quirk by mechanism", "idempotency key webhook", "quirks/webhook-replays.md", 1, nil},
		{"decision by choice", "grpc transport decision", "decisions/0001-grpc-transport.md", 1, nil},
		{"investigation by symptom", "flaky test race", "investigations/2026-01-05-flaky-tests.md", 1, nil},
		{"domain by terms", "invoice billing cycle", "domain/billing.md", 3, nil},
		{"convention by topic", "naming convention exports", "conventions/naming.md", 1, nil},
		{"entity by lifecycle", "shipment lifecycle states", "domain/shipments.md", 1, nil},
		{"second quirk", "session cookie login", "quirks/portal-logins.md", 1, nil},
		{"single shared term", "webhook", "quirks/webhook-replays.md", 3, nil},
		{"plan by shared term", "tracking", "plans/feature-tracking.md", 3, nil},
		{"package by role", "package core webhook delivery", "packages/core.md", 3, nil},
		{"component only in frontmatter", "portal", "quirks/portal-logins.md", 3, nil},
		// Fusion-pinning cases: BM25 ties that only the fusion signals decide.
		{"fusion recency tie", "invoice", "quirks/invoice-notes.md", 1, nil},
		{"fusion rule id", "PYLON042", "quirks/ratchet-drift.md", 1, nil},
		{"fusion project proximity", "delivery", "quirks/webhook-replays.md", 1, []string{"core"}},
	}

	t.Run("fusion status tie", func(t *testing.T) {
		got := filterDocs(docs, "", "", "", "", "", "webhook", 10, nil)
		live, dead := -1, -1
		for i, d := range got {
			switch d.Path {
			case "quirks/alpha-webhook.md":
				live = i
			case "decisions/0002-webhook-old.md":
				dead = i
			}
		}
		if live == -1 || dead == -1 {
			t.Fatalf("tie pair missing from results: live=%d dead=%d of %d", live, dead, len(got))
		}
		if live > dead {
			t.Fatalf("live doc ranked %d behind abandoned doc at %d; status fusion did not decide the BM25 tie", live, dead)
		}
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterDocs(docs, "", "", "", "", "", tc.query, 10, tc.projs)
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
