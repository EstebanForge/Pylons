---
title: "Shipment"
type: domain-entity
status: accepted
created: 2026-11-21
updated: 2026-11-21
owners:
  - example
components: [svc]
source_of_truth: "the transitions map in svc/shipments.go"
states: [pending, picked, shipped]
states_symbol: "repo://svc/shipments.go::ShipmentState"
machines:
  - machine: shipment
    states: [pending, picked, shipped]
    initial: [pending]
    terminal: [shipped]
    states_symbol: "repo://svc/shipments.go::ShipmentState"
    transitions_symbol: "repo://svc/shipments.go::transitions"
    transitions:
      - {from: pending, event: pick, to: picked}
      - {from: picked, event: ship, to: shipped}
avoid: [parcel, consignment, order]
description: "One shipment of goods from the warehouse to the customer."
tags: [domain]
---

# Shipment

## Definition

A shipment moves one order's goods from the warehouse to the customer.

## Invariants

- A shipment starts pending and ends shipped.
- Only the shipment machine moves the state.
- A picked shipment never returns to pending.

## Avoid

Do not call a shipment a parcel, a consignment, or an order. Those words name other things in this platform.
