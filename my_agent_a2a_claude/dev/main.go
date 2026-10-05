package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/remoteagent/v2"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
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
	// Agente remoto: Claude Code
	// Para este coordinador es un agente A2A más, igual que la panadería
	// de my_agent_a2a. No sabe que por dentro se ejecuta Claude Code.
	// ----------------------------------------------
	claudeURL := os.Getenv("CLAUDE_AGENT_URL")
	if claudeURL == "" {
		claudeURL = "http://localhost:8002"
	}

	claudeCodeConfig := remoteagent.A2AConfig{
		Name:              "claude_code",
		Description:       "Lee el código del repositorio del curso ADK y responde preguntas sobre él.",
		AgentCardProvider: remoteagent.NewAgentCardProvider(claudeURL),
	}
	claudeCode, err := remoteagent.NewA2A(claudeCodeConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente coordinador (Gemini)
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "dev_assistant",
		Model:       model,
		Description: "Asistente del curso ADK.",
		Instruction: `
			Eres el asistente del curso de ADK en Go. Responde dudas generales con
			amabilidad. Si el usuario pregunta por el código del repositorio del
			curso (qué hace un ejemplo, dónde está algo, cómo funciona un archivo),
			pásale la conversación a "claude_code".
		`,
		SubAgents: []agent.Agent{
			claudeCode,
		},
	}
	coordinator, err := llmagent.New(coordinatorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(coordinator),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
