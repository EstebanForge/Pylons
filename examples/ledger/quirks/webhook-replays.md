---
title: "Troubleshooting: webhook replays double-pick shipments"
type: quirk
status: active
created: 2026-11-21
updated: 2026-11-21
owners:
  - Pylons
package: ""
sources: []
contracts: []
guard: test
guardrail: "Wrap webhook handlers in an idempotency check before the machine call."
ai_authored_contributed: no
volume_pressure: review
description: "Provider replays called the machine twice for one delivery."
tags: [troubleshooting, example]
---

# Troubleshooting: webhook replays double-pick shipments

## Symptom

A shipment records two pick events, and the state jumps from pending to picked twice.

## Cause

The webhook provider replays delivered events. The handler called the machine on every delivery.

## Resolution

An idempotency key on the event id blocks the replay. The proof test covers the machine path.
