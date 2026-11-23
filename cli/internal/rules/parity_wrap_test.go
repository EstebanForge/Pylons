package rules

import (
	"testing"
)

// rustfmt wraps long tuple patterns and puts the value on its own line;
// both shapes are canonical rust and must extract instead of failing.
func TestRustWrappedTupleAndRHSArmsExtract(t *testing.T) {
	src := "// pylon:machine order\n" + `match ev {
    (
        OrderState::Pending,
        OrderEvent::Pay,
    ) => OrderState::Paid,
    OrderState::Paid =>
        OrderState::Shipped,
    _ => OrderState::Shipped,
}
`
	trs, err := extractRustTransitions(src, "order")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 2 {
		t.Fatalf("got %d triples, want 2: %+v", len(trs), trs)
	}
	if trs[0].from != "Pending" || trs[0].event != "Pay" || trs[0].to != "Paid" {
		t.Fatalf("tuple triple wrong: %+v", trs[0])
	}
	if trs[1].from != "Paid" || trs[1].event != "" || trs[1].to != "Shipped" {
		t.Fatalf("wrapped-RHS triple wrong: %+v", trs[1])
	}
}

// A serde rename holding a brace must not end the enum scan.
func TestRustSerdeBraceInStringExtracts(t *testing.T) {
	// The closing brace sits inside a string literal INSIDE the enum body:
	// a naive brace counter closes the block early and loses Paid, so this
	// fixture is the honesty gate for the string-aware scanner.
	src := `#[derive(Serialize)]
pub enum OrderState {
    #[serde(rename = "pending}")]
    Pending,
    Paid,
}
`
	cases, err := builtinParityExtractors["rust"](src, "OrderState")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2: %+v", len(cases), cases)
	}
}

// PHP transitions carry trailing comments; a comment holding => must
// never make the entry unparseable.
func TestPhpTrailingCommentExtracts(t *testing.T) {
	src := "<?php\nclass OrderMachine {\n\t// pylon:machine order\n\tpublic const MAP = [\n\t\t'pending' => [\n\t\t\t'pay' => 'paid',\n\t\t],\n\t\t'paid' => [\n\t\t\t'ship' => 'done', // pending => done\n\t\t],\n\t];\n}\n"
	trs, err := extractPhpTransitions(src, "order")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 2 || trs[1].from != "paid" || trs[1].to != "done" {
		t.Fatalf("triples wrong: %+v (%v)", trs, err)
	}
}

// Go states may be raw-string literals; both extraction sides must read
// the same token or the comparison always mismatches.
func TestGoRawStringStatesMatch(t *testing.T) {
	src := "package s\n\ntype State string\n\ntype Event string\n\nconst (\n\tStateNew State = `new`\n\tStateRun State = `run`\n)\n\nconst (\n\tEventGo Event = `go`\n)\n\n// pylon:machine t\nvar transitions = map[State]map[Event]State{\n\tStateNew: {EventGo: StateRun},\n}\n"
	trs, err := extractGoTransitions(src, "t")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(trs) != 1 || trs[0].from != "new" || trs[0].to != "run" {
		t.Fatalf("triples wrong: %+v", trs)
	}
	srcEnum := "package s\n\ntype State string\n\nconst (\n\tStateNew State = `new`\n)\n"
	cases, err := builtinParityExtractors["go"](srcEnum, "State")
	if err != nil {
		t.Fatalf("enum extract: %v", err)
	}
	if len(cases) != 1 || cases[0].value != "new" || !cases[0].valued {
		t.Fatalf("cases wrong: %+v", cases)
	}
}

// States split across two const blocks all count; the last block must
// not overwrite the first.
func TestGoMultiBlockStatesAppend(t *testing.T) {
	src := "package s\n\ntype State string\n\nconst (\n\tStateA State = \"a\"\n)\n\nconst (\n\tStateB State = \"b\"\n)\n"
	cases, err := builtinParityExtractors["go"](src, "State")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2 (both const blocks): %+v", len(cases), cases)
	}
}

// Integer-backed PHP and TypeScript enums carry valued cases.
func TestIntBackedEnumsExtractValued(t *testing.T) {
	php := "<?php\nenum IntStatus: int\n{\n\tcase Open = 0;\n\tcase Closed = 1;\n}\n"
	cases, err := builtinParityExtractors["php"](php, "IntStatus")
	if err != nil {
		t.Fatalf("php extract: %v", err)
	}
	if len(cases) != 2 || !cases[0].valued || cases[0].value != "0" {
		t.Fatalf("php cases wrong: %+v", cases)
	}
	ts := "export enum TsNum {\n\tOpen = 0,\n\tClosed = 1,\n}\n"
	cases, err = builtinParityExtractors["typescript"](ts, "TsNum")
	if err != nil {
		t.Fatalf("ts extract: %v", err)
	}
	if len(cases) != 2 || !cases[0].valued || cases[0].value != "0" {
		t.Fatalf("ts cases wrong: %+v", cases)
	}
}

// Comment apostrophes are prose, not quotes: an enum whose comments
// carry one must still extract.
func TestCommentApostrophesDoNotOpenQuotes(t *testing.T) {
	rust := "pub enum State {\n    // don't rename these states\n    Idle,\n    Busy,\n}\n"
	cases, err := builtinParityExtractors["rust"](rust, "State")
	if err != nil {
		t.Fatalf("rust comment apostrophe must not break extraction: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("rust got %d cases, want 2", len(cases))
	}

	php := "<?php\nenum S: string {\n    // it's the idle state marker\n    case Idle = 'idle';\n    case Busy = 'busy';\n}\n"
	cases, err = builtinParityExtractors["php"](php, "S")
	if err != nil {
		t.Fatalf("php comment apostrophe must not break extraction: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("php got %d cases, want 2", len(cases))
	}
}

// An apostrophe in prose near the enum never corrupts block matching:
// the canonical member parser still judges what is inside.
func TestRustApostropheNearEnumIsInert(t *testing.T) {
	// The apostrophe sits inside the enum body: a naive quote-span opener
	// would swallow the remaining cases, so this fixture gates the
	// line-bounded single-quote heuristic.
	rust := "pub enum State {\n    // a borrowed handle wraps &'a str\n    Idle,\n    Busy,\n}\n"
	cases, err := builtinParityExtractors["rust"](rust, "State")
	if err != nil {
		t.Fatalf("nearby apostrophe prose must not break extraction: %v", err)
	}
	if len(cases) != 2 {
		t.Fatalf("got %d cases, want 2", len(cases))
	}
}

// Lowercase bare identifiers in a parenthesized pattern are Rust
// bindings, not states: (s, e) => Err(...) is a catch-all and must
// fall through instead of extracting a phantom triple.
func TestRustTupleBindingsFallThrough(t *testing.T) {
	src := "// pylon:machine sync\nimpl SyncState {\n    fn next(&self, e: Event) -> SyncState {\n        match (self, e) {\n            (State::Idle, Event::Pull) => State::Pulling,\n            (s, e) => Err(format!(\"x\")),\n            (other, ev) => Ok(other),\n            (s, _) => Err(no),\n            _ => State::Idle,\n        }\n    }\n}\n"
	trs, err := extractRustTransitions(src, "sync")
	if err != nil {
		t.Fatalf("binding arms must fall through: %v", err)
	}
	if len(trs) != 1 || trs[0] != (parityTransition{from: "Idle", event: "Pull", to: "Pulling"}) {
		t.Fatalf("want only the canonical triple, got %+v", trs)
	}
}
