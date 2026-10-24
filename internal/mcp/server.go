// Package mcp implements a minimal MCP stdio JSON-RPC transport.
package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"log"

	"example.com/cx/internal/engine"
)

// Server speaks newline-delimited JSON-RPC on stdio.
type Server struct {
	eng *engine.Engine
	log *log.Logger
}

// New constructs an MCP server bound to eng.
func New(eng *engine.Engine, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}
	return &Server{eng: eng, log: logger}
}

// Run reads requests from in and writes responses to out. Logs go to logger only.
func (s *Server) Run(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			s.log.Printf("mcp bad json: %v", err)
			continue
		}
		method, _ := req["method"].(string)
		id := req["id"]
		// Notifications have no response.
		if id == nil && method != "" {
			continue
		}
		resp := s.handle(method, req["params"], id)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

func (s *Server) handle(method string, params any, id any) map[string]any {
	switch method {
	case "initialize":
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "cx", "version": "0.1.0"},
			},
		}
	case "tools/list":
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]any{
				"tools": []map[string]any{
					tool("find", "Lexical/hybrid/semantic code search", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
							"mode":  map[string]any{"type": "string"},
						},
						"required": []string{"query"},
					}),
					tool("references", "Find references to a symbol", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"symbol": map[string]any{"type": "string"},
						},
						"required": []string{"symbol"},
					}),
					tool("endpoints", "List Spring HTTP endpoints", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"method": map[string]any{"type": "string"},
							"path":   map[string]any{"type": "string"},
						},
					}),
					tool("config", "Resolve Spring configuration keys", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"key":     map[string]any{"type": "string"},
							"profile": map[string]any{"type": "string"},
						},
						"required": []string{"key"},
					}),
					tool("context", "Bounded symbol context", map[string]any{
						"type": "object",
						"properties": map[string]any{
							"symbol": map[string]any{"type": "string"},
							"budget": map[string]any{"type": "number"},
						},
						"required": []string{"symbol"},
					}),
				},
			},
		}
	case "tools/call":
		return s.callTool(params, id)
	default:
		return map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error":   map[string]any{"code": -32601, "message": "method not found: " + method},
		}
	}
}

func tool(name, desc string, schema map[string]any) map[string]any {
	return map[string]any{
		"name":        name,
		"description": desc,
		"inputSchema": schema,
	}
}

func (s *Server) callTool(params any, id any) map[string]any {
	pm, _ := params.(map[string]any)
	name, _ := pm["name"].(string)
	args, _ := pm["arguments"].(map[string]any)
	var payload any
	var err error
	switch name {
	case "find":
		q, _ := args["query"].(string)
		mode, _ := args["mode"].(string)
		if mode == "" {
			mode = "lexical"
		}
		payload, err = s.eng.Search(q, mode, 10)
	case "references":
		sym, _ := args["symbol"].(string)
		payload = s.eng.References(sym)
	case "endpoints":
		payload = s.eng.Endpoints(strArg(args, "method"), strArg(args, "path"))
	case "config":
		payload = s.eng.Config(strArg(args, "key"), strArg(args, "profile"))
	case "context":
		budget := 500
		if n, ok := args["budget"].(float64); ok && n > 0 {
			budget = int(n)
		}
		payload = s.eng.Context(strArg(args, "symbol"), budget)
	default:
		return map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32601, "message": "unknown tool " + name},
		}
	}
	if err != nil {
		return map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32000, "message": err.Error()},
		}
	}
	text, _ := json.Marshal(payload)
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(text)},
			},
		},
	}
}

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, _ := args[key].(string)
	return v
}
