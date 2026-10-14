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

	modelName := "gemini-3.8-flash"

	clientConfig := genai.ClientConfig{
		APIKey: apiKey,
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente coordinador de la cafetería
	// El agente remoto se usa igual que un sub-agente local.
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "cafe_coordinator",
		Model:       model,
		Description: "Atiende a los clientes de la cafetería.",
		Instruction: `
			Eres el anfitrión de una cafetería. Atiende al cliente con amabilidad.
			La cafetería no hornea su propio pan: lo compra a una panadería externa.
			Si el cliente pregunta por panes, qué hay para comer o sus precios,
			pásale la conversación al "bakery_agent".
		`,
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
