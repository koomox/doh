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
)

var (
	seed           = time.Now().UnixNano()
	defaultTimeout = 5 * time.Second
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

func newClient() *http.Client {
	transport := &http.Transport{
		IdleConnTimeout:       defaultTimeout,
		DisableCompression:    true,
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