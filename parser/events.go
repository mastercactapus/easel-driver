package parser

type Point struct {
	X, Y, Z float64
}

type Status string

const (
	StatusHold Status = "hold"
	StatusRun  Status = "run"
	StatusDoor Status = "door"
	StatusIdle Status = "idle"
)

type EventOK struct{}
type EventReady struct{ Identifier string }
type EventGRBLReport struct {
	Status     Status
	MachinePos Point
	WorkPos    Point
}
type EventGRBLSettings struct{ Settings string }
type EventGRBLBuildInfo struct {
	SerialNumber string
	Product      string
	Revision     string
}
type EventGRBLError struct{ Message string }
type EventGRBLAlarm struct{ Message string }
type EventUnknown struct{ Data string }
