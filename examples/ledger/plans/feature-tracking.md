---
title: "Track shipment lifecycle"
type: plan
status: accepted
created: 2026-11-21
updated: 2026-11-21
owners:
  - Pylons
ticket: [EX-1]
artifact: ""
target_release: ""
components: [svc]
links: []
description: "Ship the transition table and pin it with a proof test."
slice: true
scenarios:
  - given: "a pending shipment"
    when: "pick runs"
    expect: "the shipment is picked"
    covers: [repo://svc/shipments.go::transitions]
tags: [example]
contract:
  stable:
    - kind: go-symbol
      id: repo://svc/shipments.go::transitions
      wrong_if: "a triple changes while the ledger declares the old one"
      verified_by: "repo://svc/shipments_test.go::TestTransitionsMatchMachine"
    - kind: file
      id: repo://svc/shipments_test.go
      wrong_if: "the proof test is deleted or renamed"
  unstable: []
  editing: []
---

# Track shipment lifecycle

## Problem

Code can move a shipment between states without the ledger knowing. Nobody notices until a shipment skips a state.

## External contract (code-checked)

The transitions map in svc/shipments.go is the machine surface. Its proof test pins each triple inside the project's own suite.

## Design

The ledger declares the machine in domain/shipments.md. The parity pass compares the declared triples against the map on every lint.

## Milestones

- [x] Milestone 1: the machine lives in one map, checked by the ledger and the proof test.
