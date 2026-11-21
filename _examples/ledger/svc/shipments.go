// Package svc carries the shipment machine the example ledger checks.
package svc

// ShipmentState is the lifecycle state of one shipment.
type ShipmentState string

// ShipmentEvent triggers a lifecycle transition.
type ShipmentEvent string

const (
	ShipmentPending ShipmentState = "pending"
	ShipmentPicked  ShipmentState = "picked"
	ShipmentShipped ShipmentState = "shipped"
)

const (
	EventPick ShipmentEvent = "pick"
	EventShip ShipmentEvent = "ship"
)

// pylon:machine shipment
var transitions = map[ShipmentState]map[ShipmentEvent]ShipmentState{
	ShipmentPending: {
		EventPick: ShipmentPicked,
	},
	ShipmentPicked: {
		EventShip: ShipmentShipped,
	},
}
