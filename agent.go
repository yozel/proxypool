package proxypool

import (
	"fmt"
	"net/http"
	"time"
)

type State int

func (h State) String() string {
	switch h {
	case Unknown:
		return "UNKNOWN"
	case Ok:
		return "OK"
	case Error:
		return "ERROR"
	case Banned:
		return "BANNED"
	case OutOfDate:
		return "OUT OF DATE"
	case Unavailable:
		return "UNAVAILABLE"
	case Closed:
		return "CLOSED"
	default:
		return "UNDEFINED"
	}
}

const (
	Unknown State = iota
	Ok
	Error
	Banned
	OutOfDate
	Unavailable
	Closed
)

type Info struct {
	Name                 string `json:"name"`
	State                string `json:"state"`
	LastRequestTimestamp string `json:"last_request_timestamp"`
	Requests             int    `json:"requests"`
}

var ErrAgentClosed = fmt.Errorf("agent is closed")

type StateReport struct {
	State     State
	Message   string
	Timestamp time.Time
}

func (h StateReport) String() string {
	return fmt.Sprintf("%s: %s (%s ago)", h.State, h.Message, time.Since(h.Timestamp).Truncate(time.Second))
}

type Agent interface {
	Info() Info
	SetState(State, string)
	State() StateReport
	Do(*http.Request) (*http.Response, error)
	Close()
	LastRequestTime() time.Time
}
