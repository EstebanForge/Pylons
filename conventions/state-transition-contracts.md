---
title: "State transition contracts"
type: convention
slug: conv.state-transition-contracts
level: MUST
status: accepted
created: 2026-11-21
updated: 2026-11-21
owners:
  - estebanforge
ticket: []
applies_to: ["all"]
supersedes: []
artifact: ""
description: "Every lifecycle gets one formal machine declaration and one code transition map, kept in parity by lint --parity."
tags: [conventions]
---

# State transition contracts

## Rule

Declare every lifecycle as a formal machine. The domain entity in `domain/` carries one `machines:` entry per lifecycle, with the state set, the transition triples, the initial states, and the terminal states. The code carries one transition map per machine, marked with the machine id above the map. The two sides must agree exactly; `pylon lint --parity` enforces it.

A machine is the only sanctioned write surface for the lifecycle. Persisted state moves only through the transition function. A direct write of the state key outside the machine surface is a violation.

## Rationale

State machines rot quietly: a case gets added in code, the doc stays stale, and the next reader trusts a lie. A formal declaration plus a deterministic comparison catches the drift at lint time, not at incident time. The machine id ties the declaration to the code side, so neither half can drift without the comparison noticing.
