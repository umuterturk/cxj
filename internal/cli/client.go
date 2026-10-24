// Package cli talks to a running cx HTTP server for agent-friendly commands.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client is a thin HTTP client for /v1 APIs.
type Client struct {
	Base   string
	Out    io.Writer
	Err    io.Writer
	Client *http.Client
}

// New returns a client targeting base (e.g. http://127.0.0.1:8765).
func New(base string) *Client {
	return &Client{
		Base:   strings.TrimRight(base, "/"),
		Out:    os.Stdout,
		Err:    os.Stderr,
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(path string) ([]byte, int, error) {
	resp, err := c.Client.Get(c.Base + path)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) writeJSON(body []byte) int {
	_, _ = c.Out.Write(body)
	if len(body) == 0 || body[len(body)-1] != '\n' {
		_, _ = c.Out.Write([]byte("\n"))
	}
	return 0
}

func (c *Client) failHTTP(code int, body []byte) int {
	fmt.Fprintf(c.Err, "request failed: HTTP %d: %s\n", code, strings.TrimSpace(string(body)))
	return 1
}

// Status prints /v1/status.
func (c *Client) Status(asJSON bool) int {
	body, code, err := c.get("/v1/status")
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var st map[string]any
	if err := json.Unmarshal(body, &st); err != nil {
		return c.writeJSON(body)
	}
	fmt.Fprintf(c.Out, "status=%v indexed_files=%v indexed_java_files=%v generation=%v semantic_state=%v\n",
		st["status"], st["indexed_files"], st["indexed_java_files"], st["generation"], st["semantic_state"])
	return 0
}

// Find searches symbols/text.
func (c *Client) Find(query, mode string, limit int, asJSON bool) int {
	if mode == "" {
		mode = "hybrid"
	}
	if limit <= 0 {
		limit = 10
	}
	path := "/v1/search?q=" + url.QueryEscape(query) + "&mode=" + url.QueryEscape(mode) + "&limit=" + strconv.Itoa(limit)
	body, code, err := c.get(path)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var parsed struct {
		Results []struct {
			Path    string  `json:"path"`
			Line    int     `json:"line"`
			Symbol  string  `json:"symbol"`
			Kind    string  `json:"kind"`
			Match   string  `json:"match"`
			Score   float64 `json:"score"`
			Snippet string  `json:"snippet"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	for _, r := range parsed.Results {
		fmt.Fprintf(c.Out, "%s:%d\t%s\t%s\t%s\n", r.Path, r.Line, r.Symbol, r.Kind, r.Match)
	}
	return 0
}

// References looks up symbol uses.
func (c *Client) References(symbol string, asJSON bool) int {
	body, code, err := c.get("/v1/references?symbol=" + url.QueryEscape(symbol))
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var parsed struct {
		Results []struct {
			Path      string `json:"path"`
			Line      int    `json:"line"`
			Target    string `json:"target"`
			Certainty string `json:"certainty"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &parsed)
	for _, r := range parsed.Results {
		fmt.Fprintf(c.Out, "%s:%d\t%s\t%s\n", r.Path, r.Line, r.Target, r.Certainty)
	}
	return 0
}

// Endpoints lists Spring HTTP mappings.
func (c *Client) Endpoints(method, route string, asJSON bool) int {
	q := url.Values{}
	if method != "" {
		q.Set("method", method)
	}
	if route != "" {
		q.Set("path", route)
	}
	path := "/v1/endpoints"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	body, code, err := c.get(path)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var parsed struct {
		Results []struct {
			Path       string `json:"path"`
			Line       int    `json:"line"`
			Symbol     string `json:"symbol"`
			HTTPMethod string `json:"http_method"`
			Route      string `json:"route"`
			Certainty  string `json:"certainty"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &parsed)
	for _, r := range parsed.Results {
		fmt.Fprintf(c.Out, "%s %s\t%s:%d\t%s\t%s\n", r.HTTPMethod, r.Route, r.Path, r.Line, r.Symbol, r.Certainty)
	}
	return 0
}

// Beans lists Spring beans for a type.
func (c *Client) Beans(typ string, asJSON bool) int {
	body, code, err := c.get("/v1/beans?type=" + url.QueryEscape(typ))
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var parsed struct {
		Results []struct {
			Bean         string   `json:"bean"`
			Symbol       string   `json:"symbol"`
			InjectedInto []string `json:"injected_into"`
			Certainty    string   `json:"certainty"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &parsed)
	for _, r := range parsed.Results {
		fmt.Fprintf(c.Out, "%s\t%s\t%s\t%v\n", r.Bean, r.Symbol, r.Certainty, r.InjectedInto)
	}
	return 0
}

// Config resolves a configuration key.
func (c *Client) Config(key, profile string, asJSON bool) int {
	q := url.Values{"key": {key}}
	if profile != "" {
		q.Set("profile", profile)
	}
	body, code, err := c.get("/v1/config?" + q.Encode())
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	if asJSON {
		return c.writeJSON(body)
	}
	var parsed struct {
		RepositoryValue string   `json:"repository_value"`
		Resolution      string   `json:"resolution"`
		Bindings        []string `json:"bindings"`
		Definitions     []struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Value   string `json:"value"`
			Profile string `json:"profile"`
		} `json:"definitions"`
	}
	_ = json.Unmarshal(body, &parsed)
	fmt.Fprintf(c.Out, "resolution=%s repository_value=%s bindings=%v\n", parsed.Resolution, parsed.RepositoryValue, parsed.Bindings)
	for _, d := range parsed.Definitions {
		fmt.Fprintf(c.Out, "  %s:%d\t%s\tprofile=%s\n", d.Path, d.Line, d.Value, d.Profile)
	}
	return 0
}

// Context fetches bounded symbol context.
func (c *Client) Context(symbol string, budget int, asJSON bool) int {
	if budget <= 0 {
		budget = 500
	}
	path := "/v1/context?symbol=" + url.QueryEscape(symbol) + "&budget=" + strconv.Itoa(budget)
	body, code, err := c.get(path)
	if err != nil {
		fmt.Fprintln(c.Err, err)
		return 1
	}
	if code != 200 {
		return c.failHTTP(code, body)
	}
	return c.writeJSON(body) // context is nested; always JSON-shaped
}
