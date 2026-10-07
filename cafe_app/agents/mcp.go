package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/v2/auth"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

// cafeMCP conecta con el MCP de la cafetería (cafe_app/mcp), que exige un token bearer.
func cafeMCP(url, token string) (tool.Toolset, error) {
	transport := mcp.StreamableClientTransport{
		Endpoint:             url,
		DisableStandaloneSSE: true, // el servidor es stateless
	}

	toolsetConfig := mcptoolset.Config{
		Transport: &transport,
		Auth:      auth.StaticToken(token),
	}
	return mcptoolset.New(toolsetConfig)
}
