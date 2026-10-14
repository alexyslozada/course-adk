package main

import (
	"os/exec"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/mcptoolset"
)

// MCPFilesystemStdio es el servidor MCP oficial de archivos. Corre en tu
// máquina como un proceso hijo y el agente le habla por stdin/stdout.
type MCPFilesystemStdio struct {
	Dir string
}

func NewMCPFilesystemStdio(dir string) MCPFilesystemStdio {
	if dir == "" {
		panic("La carpeta del MCP de archivos no puede estar vacía")
	}

	return MCPFilesystemStdio{Dir: dir}
}

func (this MCPFilesystemStdio) ToolSet() (tool.Toolset, error) {
	// El servidor solo puede tocar las carpetas que le pasamos como argumento.
	dir, err := filepath.Abs(this.Dir)
	if err != nil {
		return nil, err
	}

	transport := mcp.CommandTransport{
		Command: exec.Command("npx", "-y", "@modelcontextprotocol/server-filesystem", dir),
	}

	toolsetConfig := mcptoolset.Config{
		Transport: &transport,
	}

	return mcptoolset.New(toolsetConfig)
}
