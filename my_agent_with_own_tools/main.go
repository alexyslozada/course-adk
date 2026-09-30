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
	// Creación del agente
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "research_agent",
		Model:       model,
		Description: "Ayudas al usuario a buscar datos en internet",
		Instruction: `
			Eres un asistente útil que le permite buscar en internet las consultas que hace 
			el usuario, y también puedes responder por la hora actual de una ciudad. 
			**Regla de tiempo:** Siempre debes utilizar la herramienta "get_current_time" 
			para responder cualquier pregunta sobre la hora actual en una ciudad. 
			Si la herramienta devuelve un error, no está disponible, o no conoce la zona horaria, 
			**tienes estrictamente prohibido calcular la hora por ti mismo o inventarla**. 
			En su lugar, debes responderle al usuario que no puedes determinar la hora actual 
			para esa ciudad debido a una limitación técnica.
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
