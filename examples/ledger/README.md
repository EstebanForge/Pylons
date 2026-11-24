# Example ledger

A miniature pylon showing the declared formats end to end: a domain entity with a machine, an accepted plan with contracts and scenarios, a quirk, and the Go code they check.

Run the checks from your pylon root:

    ./bin/pylon lint --pylons examples/ledger --parity
    ./bin/pylon brief --pylons examples/ledger

The ledger lints with zero findings. Change a triple in `svc/shipments.go` without updating the declaration, and lint exits 1 with `PYLON054 TRANSITION_MISMATCH`.

| File | Demonstrates |
|---|---|
| [domain/shipments.md](domain/shipments.md) | a `machines:` declaration, state sets, and an `avoid:` anti-vocabulary |
| [plans/feature-tracking.md](plans/feature-tracking.md) | contracts, `verified_by:` pointers, `slice:`, and `scenarios:` |
| [quirks/webhook-replays.md](quirks/webhook-replays.md) | the closed-token postmortem fields |
| [svc/shipments.go](svc/shipments.go) | the canonical Go machine shape |

Contracts resolve against the working tree because the example sets no `ref:`. Set `ref:` on a project in `manifests/pylons.yaml` to read a committed tree instead.

To drive the machine through an external plugin instead of the built-in Go profile, copy `pylons/extensions/_examples/pylon-ext-parity` into this ledger's own `extensions/` directory, bind `use: external:parity` for a `.demo` profile as `pylons/pylons-docs/parity-spec.md` describes, and point `transitions_symbol` at a marked `.demo` file.
