package main

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/v2/auth"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

type MCPGithubHTTP struct {
	Token string
}

func NewMCPGithubHTTP(token string) MCPGithubHTTP {
	if token == "" {
		panic("El token de github no puede estar vacío")
	}

	return MCPGithubHTTP{Token: token}
}

func (this MCPGithubHTTP) ToolSet() (tool.Toolset, error) {
	transport := mcp.StreamableClientTransport{
		Endpoint:             "https://api.githubcopilot.com/mcp",
		DisableStandaloneSSE: true, // Servidor stateless
	}

	toolsetConfig := mcptoolset.Config{
		Transport: &transport,
		Auth:      auth.StaticToken(this.Token),
		// Para un header propio: auth.APIKey("X-Api-Key", os.Getenv("My-Key"))
	}

	return mcptoolset.New(toolsetConfig)
}
