package svc

import "testing"

// TestTransitionsMatchMachine pins each declared triple. The ledger's
// parity pass reads the map; this test keeps it honest inside the
// project's own suite.
func TestTransitionsMatchMachine(t *testing.T) {
	tests := []struct {
		from  ShipmentState
		event ShipmentEvent
		want  ShipmentState
	}{
		{ShipmentPending, EventPick, ShipmentPicked},
		{ShipmentPicked, EventShip, ShipmentShipped},
	}
	for _, tc := range tests {
		if got := transitions[tc.from][tc.event]; got != tc.want {
			t.Errorf("%s on %s: got %s, want %s", tc.from, tc.event, got, tc.want)
		}
	}
}
