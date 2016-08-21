package machine

import (
	"io"
	"strings"
	"time"

	"github.com/mastercactapus/easel-driver/parser"
)

const MaxBytes = 127

type RunState string

var zeroTime time.Time

const (
	RunStateRunning        RunState = "RUNNING"
	RunStatePausing        RunState = "PAUSING"
	RunStatePausedDoorOpen RunState = "PAUSED_DOOR_OPEN"
	RunStatePaused         RunState = "PAUSED"
	RunStateResuming       RunState = "RESUMING"
	RunStateNone           RunState = ""
)

type Machine struct {
	gcodeQueue            []string
	consoleQueue          []string
	bufferQueue           []string
	LastRunCommand        string
	CompletedCommandCount int
	IsRunning             bool
	IsStopping            bool
	IsConnected           bool
	MachineIdentification string
	CurrentPosition       *Position
	StartRunTime          time.Time
	Config                *parser.Config
	RunState              RunState

	rwc   io.WriteCloser
	p     *parser.Parser
	hTick *time.Ticker

	reqLock chan struct{}

	C chan interface{}
}

func NewMachine(port io.WriteCloser) *Machine {
	m := &Machine{
		gcodeQueue:   make([]string, 0, 1000),
		consoleQueue: make([]string, 0, 1000),
		bufferQueue:  make([]string, 0, 1000),
		rwc:          port,
		reqLock:      make(chan struct{}),
		C:            make(chan interface{}, 10),
	}
	m.reset()
	return m
}
func (m *Machine) loop() {
	var parserEvent interface{}

	conch := struct{}{}
	for {
		select {
		case m.reqLock <- conch:
			conch = <-m.reqLock
		case parserEvent = <-m.p.C:
			switch e := parserEvent.(type) {
			case *parser.EventOK:
				m.onProcessCommand()
			case *parser.EventReady:
				m.onMachineConnected(e.Identifier)
			case *parser.EventGRBLReport:
				m.onStatus(e.Status)
				m.onPosition(&Position{Machine: e.MachinePos, Work: e.WorkPos})
			case *parser.EventGRBLSettings:
				m.onSettings(e.Settings)
			case *parser.EventGRBLAlarm:
				m.onGRBLAlarm(e.Message)
			case *parser.EventGRBLError:
				m.onGRBLError(e.Message)
			case *parser.EventGRBLBuildInfo:
				m.onReceiveMachineType(e.Product, e.Revision)
				m.onReceiveSerialNumber(e.SerialNumber)
			}
		}
	}
}
func cloneStrings(s []string) []string {
	out := make([]string, len(s))
	copy(out, s)
	return out
}
func (m *Machine) onPortOpened() {
	m.sendInstruction(InstructionFlush)
}
func (m *Machine) stopHeartbeat() {
	m.hTick.Stop()
}
func (m *Machine) onPortClosed() {
	m.stopHeartbeat()
	m.IsConnected = false
	m.reportRunTime()
	m.C <- &EventPortLost{
		MachineState: *m.getState(),
		SenderNote:   "Machine disconnected",
	}
	m.reset()
}

func (m *Machine) onReceiveMachineType(product, revision string) {
	m.C <- &EventMachineType{Type: MachineType{Product: product, Revision: revision}}
}
func (m *Machine) onReceiveSerialNumber(serialNumber string) {
	m.C <- &EventSerialNumber{SerialNumber: serialNumber}
}
func (m *Machine) onGRBLAlarm(message string) {
	m.C <- &EventGRBLAlarm{Message: message}
}
func (m *Machine) onGRBLError(message string) {
	m.C <- &EventGRBLError{Message: message}
}

func (m *Machine) onSettings(settings string) {
	m.C <- &EventGRBLSettings{Settings: settings}
}

func (m *Machine) transistionRunState(nextState RunState) {
	if nextState == RunStateNone {
		return
	}
	if m.IsRunning && m.RunState == RunStateRunning {
		m.reportRunTime()
	} else if m.IsRunning && nextState == RunStateRunning {
		m.StartRunTime = time.Now()
	}
	m.RunState = nextState
	m.enteredRunState(m.RunState)
}
func (m *Machine) paused() {
	m.C <- &EventPaused{PercentComplete: m.percentComplete()}
}
func (m *Machine) resumed() {
	m.fillCommandBuffer()
	m.C <- &EventResumed{PercentComplete: m.percentComplete()}
}
func (m *Machine) enteredRunState(r RunState) {
	switch r {
	case RunStatePausing, RunStatePausedDoorOpen, RunStatePaused:
		m.paused()
	case RunStateResuming, RunStateRunning:
		m.resumed()
	}
}
func (m *Machine) onStatus(status parser.Status) {
	if m.IsRunning {
		m.transistionRunState(statusTransition(m.RunState, status))
	}
	m.C <- &EventStatus{Status: status}
}
func (m *Machine) onPosition(pos *Position) {
	m.CurrentPosition = pos
	m.C <- &EventPosition{Position: *pos}
}

func (m *Machine) onMachineConnected(id string) {
	m.MachineIdentification = id
	m.IsConnected = true
	m.startHeartbeat()
	m.sendInstruction(InstructionReadSerialNumber)
	m.C <- EventConnected{}
}
func (m *Machine) onProcessCommand() {
	m.LastRunCommand = m.bufferQueue[0]
	m.bufferQueue = m.bufferQueue[1:]
	m.CompletedCommandCount++
	m.fillCommandBuffer()

	if m.IsRunning && m.RunState == RunStateRunning {
		m.reportJobStatus()
		if m.unprocessedCommandCount() == 0 {
			m.IsRunning = false
			m.reportRunTime()
		}
	}
}

func (m *Machine) startHeartbeat() {
	m.hTick = time.NewTicker(time.Millisecond * 500)
	go func() {
		for range m.hTick.C {
			m.sendInstruction(InstructionStatus)
		}
	}()
}
func (m *Machine) reportRunTime() {
	if m.StartRunTime != zeroTime {
		m.C <- &EventRunTime{Start: m.StartRunTime, End: time.Now()}
		m.StartRunTime = zeroTime
	}
}
func (m *Machine) nextCommand() string {
	if len(m.consoleQueue) > 0 {
		return m.consoleQueue[0]
	} else if m.IsRunning && m.RunState == RunStateRunning && len(m.gcodeQueue) > 0 {
		return m.gcodeQueue[0]
	}

	return ""
}
func (m *Machine) roomInBufferForNextCommand() bool {
	potential := append(m.bufferQueue, m.nextCommand())
	bytes := len([]byte(strings.Join(potential, "\n") + "\n"))
	return bytes <= MaxBytes
}
func (m *Machine) unprocessedCommandCount() int {
	return len(m.consoleQueue) + len(m.gcodeQueue) + len(m.bufferQueue)
}

func (m *Machine) running() {
	m.C <- &EventProgress{PercentComplete: m.percentComplete()}
}
func (m *Machine) reportRunState() {
	m.C <- &EventRunState{RunState: m.RunState}
}
func (m *Machine) stopping() {
	m.C <- &EventStopping{}
}
func (m *Machine) ready() {
	m.C <- &EventReady{}
}
func (m *Machine) reportJobStatus() {
	if m.IsRunning {
		m.reportRunState()
		switch m.RunState {
		case RunStateRunning, RunStateResuming:
			m.running()
		case RunStatePaused, RunStatePausing, RunStatePausedDoorOpen:
			m.paused()
		}
	} else if m.IsStopping {
		m.stopping()
	} else if m.IsConnected {
		m.ready()
	}
}
func (m *Machine) dequeueNextCommand() string {
	if len(m.consoleQueue) > 0 {
		line := m.consoleQueue[0]
		m.consoleQueue = m.consoleQueue[1:]
		return line
	} else if len(m.gcodeQueue) > 0 {
		line := m.gcodeQueue[0]
		m.gcodeQueue = m.gcodeQueue[1:]
		return line
	}
	return ""
}

func (m *Machine) sendLine(line string) {
	io.WriteString(m.rwc, line+"\n")
}

func (m *Machine) fillCommandBuffer() {
	for m.nextCommand() != "" && m.roomInBufferForNextCommand() {
		line := m.dequeueNextCommand()
		m.bufferQueue = append(m.bufferQueue, line)
		m.sendLine(line)
	}
}
func (m *Machine) resetQueue() {
	m.consoleQueue = m.consoleQueue[:0]
	m.gcodeQueue = m.gcodeQueue[:0]
	m.bufferQueue = m.bufferQueue[:0]
}
func (m *Machine) percentComplete() float64 {
	return float64(m.CompletedCommandCount) / float64(m.unprocessedCommandCount())
}

func (m *Machine) reset() {
	m.IsRunning = false
	m.RunState = RunStateRunning
	m.resetQueue()
	m.CompletedCommandCount = 0
}

func (m *Machine) sendInstruction(instruction Instruction) {
	gcode := m.Config.GCode[string(instruction)]
	switch instruction {
	case InstructionFlush:
		m.resetQueue()
		fallthrough
	case InstructionPause, InstructionResume, InstructionStatus:
		io.WriteString(m.rwc, gcode)
	default:
		if gcode != "" {
			m.enqueueCommand(gcode)
		}
	}
}
func (m *Machine) enqueueCommand(line string) {
	m.consoleQueue = append(m.consoleQueue, line)
	m.fillCommandBuffer()
}

func (m *Machine) lock() {
	<-m.reqLock
}
func (m *Machine) unlock() {
	m.reqLock <- struct{}{}
}
func (m *Machine) GetMachineIdentification() string {
	m.lock()
	defer m.unlock()
	if m.IsConnected {
		return m.MachineIdentification
	}

	return ""
}

func (m *Machine) getState() *MachineState {
	return &MachineState{
		CompletedCommands: m.CompletedCommandCount,
		PendingCommands:   len(m.consoleQueue) + len(m.gcodeQueue),
		CurrentPosition:   *m.CurrentPosition,
		LastInstruction:   m.LastRunCommand,
		ActiveBuffer:      cloneStrings(m.bufferQueue),
		Running:           m.IsRunning,
		Paused:            m.RunState == RunStatePaused,
		Stopping:          m.IsStopping,
	}
}
func (m *Machine) StreamGCodeLines(lines []string) {
	m.lock()
	defer m.unlock()
	m.gcodeQueue = cloneStrings(lines)
	m.IsRunning = true
	m.RunState = RunStateRunning
	m.CompletedCommandCount = 0
	m.StartRunTime = time.Now()
	m.reportJobStatus()
	m.fillCommandBuffer()
}
func (m *Machine) CurrentState() *MachineState {
	m.lock()
	defer m.unlock()
	return m.getState()
}
func (m *Machine) RequestSettings() {
	m.lock()
	defer m.unlock()

	m.sendInstruction(InstructionSettings)
}
func (m *Machine) EnqueueCommand(line string) {
	m.lock()
	defer m.unlock()
	m.enqueueCommand(line)
}
func (m *Machine) Disconnect() {
	m.lock()
	defer m.unlock()
	m.stopHeartbeat()
	m.rwc.Close()
	m.IsConnected = false
	m.reset()
}
func (m *Machine) ReportJobStatus() {
	m.lock()
	defer m.unlock()
	m.reportJobStatus()
}
func (m *Machine) easelAction(action Action) {
	m.transistionRunState(actionTransition(m.RunState, action))
}
func (m *Machine) Pause() {
	m.lock()
	defer m.unlock()
	m.sendInstruction(InstructionPause)
	m.easelAction(ActionPause)
}
func (m *Machine) Resume() {
	m.lock()
	defer m.unlock()
	m.sendInstruction(InstructionResume)
	m.easelAction(ActionResume)
}
func (m *Machine) Stop() {
	m.lock()
	if !m.IsRunning {
		m.unlock()
		return
	}
	m.IsStopping = true
	m.stopping()
	m.reset()
	m.sendInstruction(InstructionPause)
	m.unlock()
	time.Sleep(time.Second)
	m.lock()
	m.sendInstruction(InstructionFlush)
	m.unlock()
	time.Sleep(time.Second)
	m.lock()
	m.sendInstruction(InstructionResume)
	m.unlock()
	time.Sleep(time.Second)
	m.lock()
	m.sendInstruction(InstructionLiftToSafeHeight)
	m.sendInstruction(InstructionSpindleOff)
	m.sendInstruction(InstructionPark)
	m.IsStopping = false
	m.reportJobStatus()
	m.unlock()
}
func (m *Machine) Acquire(t time.Time) {
	m.lock()
	defer m.unlock()
	if m.IsRunning {
		m.C <- &EventRelease{Timestamp: t}
	}
}
func (m *Machine) SetConfig(c *parser.Config) {
	m.lock()
	defer m.unlock()
	m.p.SetConfig(c)
	m.Config = c
}
func (m *Machine) Execute(instructions []Instruction) {
	m.lock()
	defer m.unlock()
	for _, ins := range instructions {
		m.sendInstruction(ins)
	}
}
