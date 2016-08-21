package parser

type Config struct {
	Name            string
	GCode           map[string]string
	Baud            int
	Separator       string
	ReadyResponses  []string
	SuccessResponse string
}
