package machine

import "github.com/mastercactapus/easel-driver/parser"

func statusTransition(state RunState, status parser.Status) RunState {
	switch {
	case state == RunStatePausing && status == parser.StatusHold:
		return RunStatePaused
	case state == RunStatePausing && status == parser.StatusDoor:
		return RunStatePausedDoorOpen
	case state == RunStatePaused && status == parser.StatusRun:
		return RunStateRunning
	case state == RunStatePaused && status == parser.StatusDoor:
		return RunStatePausedDoorOpen
	case state == RunStatePausedDoorOpen && status == parser.StatusHold:
		return RunStatePaused
	case state == RunStatePausedDoorOpen && status == parser.StatusRun:
		return RunStateRunning
	case state == RunStateResuming && status == parser.StatusRun:
		return RunStateRunning
	case state == RunStateResuming && status == parser.StatusDoor:
		return RunStatePausedDoorOpen
	case state == RunStateRunning && status == parser.StatusHold:
		return RunStatePaused
	case state == RunStateRunning && status == parser.StatusDoor:
		return RunStatePausedDoorOpen
	}
	return RunStateNone
}

type Action string

const (
	ActionResume Action = "resume"
	ActionPause  Action = "pause"
)

func actionTransition(state RunState, action Action) RunState {
	switch {
	case state == RunStatePaused && action == ActionResume:
		return RunStateResuming
	case state == RunStateRunning && action == ActionPause:
		return RunStatePausing
	case state == RunStatePausing && action == ActionResume:
		return RunStateResuming
	case state == RunStateResuming && action == ActionPause:
		return RunStatePausing
	}
	return RunStateNone
}
