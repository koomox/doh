package doh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"
	"strings"
)

const (
	defaultTimeout = 5 * time.Second
)

var (
	seed           = time.Now().UnixNano()
	doh_json_api_url = map[string][]string {
		"cloudflare": {
			"https://1.1.1.1/dns-query",
			"https://1.0.0.1/dns-query",
		},
		"google": {
			"https://8.8.8.8/resolve",
			"https://8.8.4.4/resolve",
		},
		"quad9": {
			"https://9.9.9.9:5053/dns-query",
			"https://9.9.9.10:5053/dns-query",
			"https://9.9.9.11:5053/dns-query",
		},
	}

	dohJSONAPI []string
)

func init() {
	for i, _ := range doh_json_api_url {
		dohJSONAPI = append(dohJSONAPI, doh_json_api_url[i]...)
	}
}

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

func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			IdleConnTimeout:       defaultTimeout,
			DisableCompression:    true,
			TLSHandshakeTimeout:   defaultTimeout,
			ResponseHeaderTimeout: defaultTimeout,
			ExpectContinueTimeout: defaultTimeout,
			DisableKeepAlives:     true,
		},
	}
}

func exchangeHTTPS(name, provider string) (b []byte, err error) {
	var req *http.Request = nil
	var resp *http.Response = nil

	Url, err := url.Parse(provider)
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

	client := newClient()
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

func lookup(name, provider string) (addrs []string, err error) {
	r, err := exchangeHTTPS(name, provider)
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

func Lookup(name string)(addrs []string, err error) {
	msgQ := make(chan []string)
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	for i := 0; i < len(dohJSONAPI); i++ {
		go func(name, provider string){
			address, e := lookup(name, provider)
			if e == nil {
				cancel()
				msgQ <- append([]string{"success"}, address...)
			}
		}(name, dohJSONAPI[i])
	}
	select {
	case <-time.After(defaultTimeout):
		cancel()
		return nil, errors.New("time out i/o")
	case msg := <-msgQ:
		if strings.EqualFold(msg[0], "success") {
			addrs = append(addrs, msg[1:]...)
			return
		}
		return nil, errors.New(msg[0])
	}
}