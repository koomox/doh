package doh

import (
	"context"
	"errors"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"time"
)

type Client struct {
	client *http.Client
}

func NewClient() *Client {
	return &Client{
		client: makeClient(),
	}
}

func makeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return dialer.DialContext(ctx, network, addr)
			}

			ip, err := Lookup(host)
			if err != nil {
				return dialer.DialContext(ctx, network, addr)
			}

			r := rand.New(rand.NewSource(seed))
			i := r.Intn(len(ip))
			seed += int64(i)
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip[i], port))
		},
		TLSHandshakeTimeout:   defaultTimeout,
		ResponseHeaderTimeout: defaultTimeout,
		ExpectContinueTimeout: defaultTimeout,
		DisableKeepAlives:     true,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client
}

func (this *Client) get(req *http.Request, ctx context.Context) (buf []byte, err error) {
	r, err := this.client.Do(req.WithContext(ctx))
	if err != nil {
		return
	}

	defer r.Body.Close()
	return ioutil.ReadAll(r.Body)
}

func (this *Client) Do(req *http.Request) ([]byte, error) {
	msgQ := make(chan string)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	for i := 0; i < 20; i++ {
		go func() {
			if b, err := this.get(req, ctx); err == nil {
				cancel()
				msgQ <- string(b)
			}
		}()
	}

	select {
	case <-time.After(3000 * time.Millisecond):
		cancel()
		return nil, errors.New("time out i/o")
	case msg := <-msgQ:
		return []byte(msg), nil
	}
}
