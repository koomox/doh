package doh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"time"
)

var (
	seed = time.Now().UnixNano()
)

// Question is dns query question
type Question struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

// Answer is dns query answer
type Answer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

// Response is dns query response
type Response struct {
	Status   int        `json:"Status"`
	TC       bool       `json:"TC"`
	RD       bool       `json:"RD"`
	RA       bool       `json:"RA"`
	AD       bool       `json:"AD"`
	CD       bool       `json:"CD"`
	Question []Question `json:"Question"`
	Answer   []Answer   `json:"Answer"`
}

type Client struct {
	request *http.Request
	client  *http.Client
}

func NewClient(req *http.Request) *Client {
	return &Client{
		request: req,
		client:  createHttpClient(),
	}
}

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		IdleConnTimeout:       5 * time.Second,
		DisableCompression:    true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		DisableKeepAlives:     true,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client
}

const (
	CLOUDFLARE_DOH_URL = "https://1.1.1.1/dns-query?"
	QUAD9_DOH_URL      = "https://9.9.9.9:5053/dns-query?"
)

func exchangeHTTPS(name string) (b []byte, err error) {
	var req *http.Request = nil
	var resp *http.Response = nil

	Url, err := url.Parse(CLOUDFLARE_DOH_URL)
	if err != nil {
		return
	}

	params := url.Values{}
	params.Add("name", name)
	params.Add("type", "A")
	Url.RawQuery = params.Encode() // Escape Query Parameters
	if req, err = http.NewRequest(http.MethodGet, Url.String(), nil); err != nil {
		return
	}

	req.Header.Add("Accept", "application/dns-json")

	client := newHTTPClient()
	if resp, err = client.Do(req); err != nil {
		return
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("HTTPS server returned with non-OK code %d", resp.StatusCode)
		return
	}

	return ioutil.ReadAll(resp.Body)
}

func Lookup(name string) (addrs []string, err error) {
	r, err := exchangeHTTPS(name)
	if err != nil {
		return
	}
	resp := &Response{}
	if err = json.NewDecoder(bytes.NewBuffer(r)).Decode(resp); err != nil {
		return
	}
	if resp.Status != 0 {
		err = fmt.Errorf("%s's server IP address could not be found.", name)
		return
	}
	for _, answer := range resp.Answer {
		if net.ParseIP(answer.Data) != nil {
			addrs = append(addrs, answer.Data)
		}
	}

	return
}

func createHttpClient() *http.Client {
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
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		DisableKeepAlives:     true,
	}

	client := &http.Client{
		Transport: transport,
	}

	return client
}

func Request(req *http.Request, ctx context.Context) (buf []byte, err error) {
	client := createHttpClient()
	r, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return
	}

	defer r.Body.Close()
	return ioutil.ReadAll(r.Body)
}

func Get(req *http.Request) (string, error) {
	msgQ := make(chan string)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	for i := 0; i < 20; i++ {
		go func() {
			if b, err := Request(req, ctx); err == nil {
				cancel()
				msgQ <- string(b)
			}
		}()
	}

	select {
	case <-time.After(3000 * time.Millisecond):
		cancel()
		return "done", errors.New("time out i/o")
	case msg := <-msgQ:
		return msg, nil
	}
}
