// Copyright 2026 EstebanForge <esteban at attitude dot cl>
// SPDX-License-Identifier: Apache-2.0

package resolve

import "testing"

func TestBM25SearchRankingAndLimit(t *testing.T) {
	docs := []BM25Doc{
		{ID: "a.md", Text: "authentication token refresh session"},
		{ID: "b.md", Text: "token token token authentication flow"},
		{ID: "c.md", Text: "database migration schema"},
	}
	idx := NewBM25Index(docs)

	hits := idx.Search("token", 0)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %+v", len(hits), hits)
	}
	if hits[0].ID != "b.md" {
		t.Fatalf("expected b.md first (higher TF), got %s", hits[0].ID)
	}

	limited := idx.Search("token", 1)
	if len(limited) != 1 || limited[0].ID != "b.md" {
		t.Fatalf("expected 1 limited hit (b.md), got %+v", limited)
	}

	empty := idx.Search("nonexistent", 0)
	if len(empty) != 0 {
		t.Fatalf("expected 0 hits, got %+v", empty)
	}
}
