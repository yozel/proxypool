package proxypool

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/context"
)

const (
	MaxRetry = 3
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type Pool struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	middleware func(c *Context)
}

func New(fn func(c *Context)) *Pool {
	p := &Pool{
		agents:     make(map[string]Agent),
		middleware: fn,
	}
	return p
}

func (p *Pool) Status() []Info {
	r := make([]Info, 0, len(p.agents))
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, a := range p.agents {
		r = append(r, a.Info())
	}
	return r
}

func (p *Pool) Add(name string, agent Agent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.agents[name]; !ok {
		p.agents[name] = agent
	} else {
		log.Printf("agent %s already exists", name)
	}
}

func (p *Pool) Delete(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.agents[name]
	if !ok {
		return fmt.Errorf("agent %s not found", name)
	}
	agent.Close()
	delete(p.agents, name)
	return nil
}

func (p *Pool) List() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]string, 0, len(p.agents))
	for name := range p.agents {
		result = append(result, name)
	}
	return result
}

func (p *Pool) getOkAgents() []Agent {
	p.mu.Lock()
	defer p.mu.Unlock()
	healthyAgents := p.getAgents(Ok)
	timeoutFirst, timeoutLast := splitSlice(shuffleSlice(p.getAgents(OutOfDate)), 1)
	result := concatSlice(timeoutFirst, healthyAgents, timeoutLast)
	return result
}

func (p *Pool) getAgents(health State) []Agent {
	allAgents := values(p.agents)
	result := filter(func(a Agent) bool {
		return a.State().State == health
	}, allAgents)
	return sortSlice(result, func(a, b Agent) bool {
		return a.LastRequestTime().Before(b.LastRequestTime())
	})
}

var ErrNoHealthyAgents = fmt.Errorf("no healthy agents")

type Context struct {
	*http.Response
	Err   error
	Agent Agent
	Retry bool
	Body  []byte
}

func newContext(agent Agent, res *http.Response, err error) (*Context, error) {
	if err != nil {
		return &Context{
			Response: nil,
			Err:      err,
			Agent:    agent,
			Body:     nil,
		}, nil
	}
	if res == nil {
		return nil, errors.New("response is nil but error is also nil")
	}
	defer res.Body.Close()
	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return &Context{
		Response: res,
		Err:      nil,
		Agent:    agent,
		Body:     bodyBytes,
	}, nil
}

func (p *Pool) Do(req *http.Request) (*http.Response, error) {
	var (
		bodyBytes []byte
		err       error
	)
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
	}
	factory := func() *http.Request {
		tmp := req.Clone(req.Context())
		if bodyBytes != nil {
			tmp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
		return tmp
	}

	for i, a := range p.getOkAgents() {
		if i+1 > MaxRetry {
			log.Printf("max retry reached for %s", a.Info().Name)
			break
		}
		if i+1 > 1 {
			log.Printf("retry #%d with agent %s", i+1, a.Info().Name)
		}
		res, err := a.Do(factory())
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		c, err := newContext(a, res, err)
		if err != nil {
			return nil, err
		}
		p.middleware(c)
		if c.Retry {
			continue
		}
		if c.Err != nil {
			return nil, c.Err
		}
		res2 := &http.Response{
			Status:           c.Status,
			StatusCode:       c.StatusCode,
			Proto:            c.Proto,
			ProtoMajor:       c.ProtoMajor,
			ProtoMinor:       c.ProtoMinor,
			Header:           c.Header.Clone(),
			ContentLength:    c.ContentLength,
			TransferEncoding: c.TransferEncoding,
			Close:            false,
			Uncompressed:     c.Uncompressed,
			Trailer:          c.Trailer.Clone(),
			Request:          c.Request,
		}
		res2.Body = io.NopCloser(bytes.NewReader(c.Body))
		return res2, nil
	}
	return nil, ErrNoHealthyAgents
}
