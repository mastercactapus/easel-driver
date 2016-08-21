package parser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

func containsAny(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

var (
	isGRBLReport    = regexp.MustCompile(`^<.*>$`)
	isGRBLBuildInfo = regexp.MustCompile(`^\[.+:[\d-]+\]$`)
	isGRBLSettings  = regexp.MustCompile(`^\$\d+\s*=`)
	isGRBLError     = regexp.MustCompile(`error:(.*)`)
	isGRBLAlarm     = regexp.MustCompile(`ALARM:(.*)`)

	grblReportRx    = regexp.MustCompile(`(\w+),MPos:([-+]?[0-9]*\.?[0-9]+),([-+]?[0-9]*\.?[0-9]+),([-+]?[0-9]*\.?[0-9]+),WPos:([-+]?[0-9]*\.?[0-9]+),([-+]?[0-9]*\.?[0-9]+),([-+]?[0-9]*\.?[0-9]+)`)
	grblBuildInfoRx = regexp.MustCompile(`^\[(.+)\]$`)
)

type Parser struct {
	lines     chan string
	setConfig chan *Config
	c         *Config

	C chan interface{}
}

func NewParser(lines chan string, c *Config) *Parser {
	p := &Parser{
		lines:     lines,
		c:         c,
		setConfig: make(chan *Config),
		C:         make(chan interface{}, 100),
	}
	go p.loop()
	return p
}
func (p *Parser) loop() {
	var line string
	var c *Config
	for {
		select {
		case c = <-p.setConfig:
			p.c = c
		case line = <-p.lines:
			line = strings.TrimSpace(line)
			if containsAny(line, p.c.ReadyResponses) {
				p.C <- &EventReady{Identifier: line}
			} else if strings.Contains(line, p.c.SuccessResponse) {
				p.C <- &EventOK{}
			} else if isGRBLReport.MatchString(line) {
				report, err := parseGRBLReport(line)
				if err == nil {
					p.C <- report
				}
			} else if isGRBLSettings.MatchString(line) {
				p.C <- &EventGRBLSettings{Settings: line}
			} else if isGRBLBuildInfo.MatchString(line) {
				info, err := parseGRBLBuildInfo(line)
				if err == nil {
					p.C <- info
				}
			} else if isGRBLError.MatchString(line) {
				p.C <- &EventGRBLError{Message: line}
			} else if isGRBLAlarm.MatchString(line) {
				p.C <- &EventGRBLAlarm{Message: line}
			} else {
				p.C <- &EventUnknown{Data: line}
			}
		}
	}
}

func parseGRBLReport(line string) (*EventGRBLReport, error) {
	parts := grblReportRx.FindStringSubmatch(line)
	if parts == nil || len(parts) < 8 {
		return nil, errors.New("invalid format")
	}

	var r EventGRBLReport
	r.Status = Status(strings.ToLower(parts[1]))
	var err error
	r.MachinePos.X, err = strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return nil, err
	}
	r.MachinePos.Y, err = strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return nil, err
	}
	r.MachinePos.Z, err = strconv.ParseFloat(parts[4], 64)
	if err != nil {
		return nil, err
	}
	r.WorkPos.X, err = strconv.ParseFloat(parts[5], 64)
	if err != nil {
		return nil, err
	}
	r.WorkPos.Y, err = strconv.ParseFloat(parts[6], 64)
	if err != nil {
		return nil, err
	}
	r.WorkPos.Z, err = strconv.ParseFloat(parts[7], 64)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func parseGRBLBuildInfo(line string) (*EventGRBLBuildInfo, error) {
	parts := grblBuildInfoRx.FindStringSubmatch(line)
	if parts == nil {
		return nil, errors.New("invalid format")
	}
	var g EventGRBLBuildInfo
	fields := strings.Split(parts[1], ":")
	if len(fields) >= 4 {
		g.Product = fields[1]
		g.Revision = fields[2]
	}
	g.SerialNumber = fields[len(fields)-1]
	return &g, nil
}

func (p *Parser) SetConfig(c *Config) {
	p.setConfig <- c
}
