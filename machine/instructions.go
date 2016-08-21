package machine

type Instruction string

const (
	InstructionFlush            Instruction = "flush"
	InstructionPause            Instruction = "pause"
	InstructionResume           Instruction = "resume"
	InstructionSettings         Instruction = "settings"
	InstructionLiftToSafeHeight Instruction = "liftToSafeHeight"
	InstructionSpindleOff       Instruction = "spindleOff"
	InstructionPark             Instruction = "park"
	InstructionStatus           Instruction = "status"
	InstructionReadSerialNumber Instruction = "readSerialNumber"
)
