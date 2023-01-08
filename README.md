# proxypool

proxypool is a Go package that provides a convenient way to use a pool of HTTP proxies in your program. With proxypool, you can add multiple proxy agents to the pool and set a rate limit for each of them. You can then use the pool as a single HTTP roundtripper, making requests through a randomly selected proxy from the pool. proxypool also includes a smart retry mechanism that can automatically mark a proxy as banned when it returns a 403 Forbidden status code and try the request again using a different proxy. This helps ensure that your requests are not blocked by the target server. proxypool is a useful tool for tasks such as web scraping and bypassing IP-based rate limits.

## Usage

```go
package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/yozel/proxypool"
	"golang.org/x/time/rate"
)

var (
	proxyMap = map[string]url.URL{
		"proxy1": mustUrlMarse("socks5://user:pass@111.222.111.222:1080"),
		"proxy2": mustUrlMarse("socks5://user:pass@123.123.123.123:1080"),
		"proxy3": mustUrlMarse("socks5://user:pass@321.321.321.321:1080"),
	}
)

func main() {
	ap := proxypool.New(func(c *proxypool.Context) {
		if c.Err != nil {
			c.Agent.SetState(proxypool.Error, c.Err.Error())
			c.Retry = true
			return
		}
		if c.StatusCode == http.StatusForbidden {
			c.Agent.SetState(proxypool.Banned, "403")
			c.Retry = true
			return
		}
		c.Agent.SetState(proxypool.Ok, "")
		c.Retry = false
	})

	for k, v := range proxyMap {
		ap.Add(k, proxypool.NewProxyAgentWithLimiter(v, rate.NewLimiter(rate.Every(180*time.Second), 10)))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://httpbin.org/get", nil)
		if err != nil {
			log.Printf("error creating request: %v", err)
			continue
		}
		resp, err := ap.Do(req)
		if err != nil {
			log.Printf("error getting response: %v", err)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("error reading response: %v", err)
			continue
		}
		log.Printf("response: %s", b)
		resp.Body.Close()
	}

}

func mustUrlMarse(s string) url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return *u
}
```