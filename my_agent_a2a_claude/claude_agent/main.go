package main

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

// Este agente no usa Gemini: es un agente personalizado del ADK que, en cada
// mensaje, ejecuta Claude Code. Se expone por A2A con el launcher:
//
//	go run ./claude_agent web -port 8002 -write-timeout 5m a2a -a2a_agent_url http://localhost:8002
func main() {
	ctx := context.Background()

	// ----------------------------------------------
	// Carpeta en la que trabaja Claude Code
	// Por defecto, la raíz del repositorio del curso
	// ----------------------------------------------
	workDir := os.Getenv("CLAUDE_WORKDIR")
	if workDir == "" {
		workDir = ".."
	}
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente personalizado
	// En lugar de llmagent.New, usamos agent.New y escribimos nosotros
	// la función Run: recibe el mensaje y devuelve eventos.
	// ----------------------------------------------
	claudeAgentConfig := agent.Config{
		Name:        "claude_code",
		Description: "Claude Code en modo solo lectura: responde preguntas sobre el código del repositorio del curso ADK.",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				answer, err := AskClaude(ctx, workDir, userText(ctx))
				if err != nil {
					yield(nil, err)
					return
				}

				// La respuesta de Claude Code se devuelve como un evento del agente
				event := session.NewEvent(ctx, ctx.InvocationID())
				event.Author = ctx.Agent().Name()
				event.LLMResponse = model.LLMResponse{
					Content: genai.NewContentFromText(answer, genai.RoleModel),
				}
				yield(event, nil)
			}
		},
	}
	claudeAgent, err := agent.New(claudeAgentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(claudeAgent),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

// userText devuelve el texto del mensaje que recibió el agente.
func userText(ctx agent.InvocationContext) string {
	content := ctx.UserContent()
	if content == nil {
		return ""
	}

	var text strings.Builder
	for _, part := range content.Parts {
		text.WriteString(part.Text)
	}

	return strings.TrimSpace(text.String())
}
