package proxypool

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var _ Agent = (*ProxyAgentWithLimiter)(nil)

type ProxyAgentWithLimiter struct {
	mu              sync.RWMutex
	url             url.URL
	limiter         *rate.Limiter
	state           StateReport
	requests        int
	lastRequestTime time.Time
	client          *http.Client
	wg              sync.WaitGroup
	closed          bool
}

func NewProxyAgentWithLimiter(url url.URL, limiter *rate.Limiter) *ProxyAgentWithLimiter {
	return &ProxyAgentWithLimiter{
		url:     url,
		limiter: limiter,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(&url),
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 300 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

func (a *ProxyAgentWithLimiter) LastRequestTime() time.Time {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastRequestTime
}

func (a *ProxyAgentWithLimiter) Info() Info {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return Info{
		Name:                 a.url.Host,
		State:                fmt.Sprintf("%s, %d tokens", a.State().String(), int(a.limiter.Tokens())),
		LastRequestTimestamp: time.Since(a.lastRequestTime).Truncate(time.Second).String(),
		Requests:             a.requests,
	}
}

func (a *ProxyAgentWithLimiter) Close() {
	a.mu.Lock()
	a.closed = true
	client := a.client
	a.client = nil
	a.mu.Unlock()
	if client != nil {
		client.CloseIdleConnections()
		a.wg.Wait()
		client.CloseIdleConnections()
	}
	a.client = nil
}

func (a *ProxyAgentWithLimiter) State() StateReport {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.closed {
		return StateReport{
			State:     Closed,
			Message:   "Agent closed",
			Timestamp: time.Now(),
		}
	}
	if a.limiter.Tokens() < 1 {
		return StateReport{
			State:     Unavailable,
			Message:   "No tokens available",
			Timestamp: time.Now(),
		}
	}
	if a.state.State != Ok && time.Since(a.state.Timestamp) > 300*time.Second {
		return StateReport{
			State:     OutOfDate,
			Message:   "Out of date health report",
			Timestamp: time.Now(),
		}
	}
	return a.state
}

func (a *ProxyAgentWithLimiter) SetState(h State, msg string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = StateReport{
		State:     h,
		Message:   msg,
		Timestamp: time.Now(),
	}
}

func (a *ProxyAgentWithLimiter) Do(req *http.Request) (*http.Response, error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, ErrAgentClosed
	}
	if !a.limiter.Allow() {
		a.mu.Unlock()
		return nil, fmt.Errorf("rate limit exceeded")
	}
	a.wg.Add(1)
	defer a.wg.Done()
	if a.client == nil {
		a.client = &http.Client{ // TODO: remove this
			Transport: &http.Transport{
				Proxy: http.ProxyURL(&a.url),
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 300 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     false,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: 5 * time.Second,
		}
	}
	a.requests += 1
	a.lastRequestTime = time.Now()
	a.mu.Unlock()
	return a.client.Do(req)
}
