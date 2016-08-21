package machine

import (
	"time"

	"github.com/mastercactapus/easel-driver/parser"
)

type Position struct {
	Machine parser.Point
	Work    parser.Point
}

type MachineType struct {
	Product, Revision string
}
type MachineState struct {
	CompletedCommands int
	PendingCommands   int
	CurrentPosition   Position
	LastInstruction   string
	ActiveBuffer      []string
	Running           bool
	Paused            bool
	Stopping          bool
}

type EventGRBLAlarm struct{ Message string }
type EventGRBLError struct{ Message string }
type EventMachineType struct{ Type MachineType }
type EventSerialNumber struct{ SerialNumber string }
type EventConnected struct{}
type EventStatus struct{ Status parser.Status }
type EventPosition struct{ Position Position }
type EventReady struct{}
type EventGRBLSettings struct{ Settings string }
type EventPortLost struct {
	MachineState
	SenderNote string
}
type EventProgress struct{ PercentComplete float64 }
type EventRunState struct{ RunState RunState }
type EventRunTime struct {
	Start time.Time
	End   time.Time
}
type EventPaused struct{ PercentComplete float64 }
type EventResumed struct{ PercentComplete float64 }
type EventStopping struct{}
type EventRelease struct{ Timestamp time.Time }
