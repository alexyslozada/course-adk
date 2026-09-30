package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/adk/v2/tool/geminitool"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	// ----------------------------------------------
	// Creación del modelo
	// ----------------------------------------------
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		fmt.Println("Debes configurar GOOGLE_API_KEY")
		os.Exit(1)
	}
	githubKey := os.Getenv("GITHUB_TOKEN")
	if githubKey == "" {
		fmt.Println("Debes configurar GITHUB_TOKEN")
		os.Exit(1)
	}

	modelName := "gemini-3.5-flash-lite"

	clientConfig := genai.ClientConfig{
		APIKey: apiKey,
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Convertir la función en una function tool
	// ----------------------------------------------
	currentTimeToolConfig := functiontool.Config{
		Name:        "get_current_time",
		Description: "Devuelve la hora actual en una ciudad. Úsala cuando el usuario pregunte qué hora es en algún lugar.",
	}
	currentTimeTool, err := functiontool.New(currentTimeToolConfig, CurrentTime)
	if err != nil {
		fmt.Printf("Hubo un error al convertir la función %s a tool", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// MCP via HTTP stateless
	// ----------------------------------------------
	mcpStruct := NewMCPGithubHTTP(githubKey)
	mcpGithub, err := mcpStruct.ToolSet()
	if err != nil {
		fmt.Printf("No se pudo cargar el mcp: %s", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// MCP via HTTP con login (OAuth)
	// ----------------------------------------------
	mcpJiraStruct := NewMCPJiraHTTP("Curso ADK con Go")
	mcpJira, err := mcpJiraStruct.ToolSet()
	if err != nil {
		fmt.Printf("No se pudo cargar el mcp de Jira: %s", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del agente
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "research_agent",
		Model:       model,
		Description: "Ayudas al usuario a buscar datos en internet",
		Instruction: `
			Eres un asistente útil que le permite buscar en internet las 
			consultas que hace el usuario. 

			También puedes responder por la 
			hora actual de una ciudad. Para la hora debes utilizar la 
			tool llamada "get_current_time".

			También tienes un MCP que te permite trabajar con Github, conéctate cuando el usuario lo solicite.

			También tienes un MCP de Atlassian que te permite trabajar con Jira y Confluence, conéctate cuando el usuario lo solicite.
		`,
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
			currentTimeTool,
		},
		// Necesario para combinar una builtin tool (GoogleSearch) con function calling
		GenerateContentConfig: &genai.GenerateContentConfig{
			ToolConfig: &genai.ToolConfig{
				IncludeServerSideToolInvocations: new(true),
			},
		},
		Toolsets: []tool.Toolset{
			mcpGithub,
			mcpJira,
		},
	}

	a, err := llmagent.New(agentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(a),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
