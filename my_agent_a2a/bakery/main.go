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

// La panadería es un agente normal, como los primeros del curso.
// No tiene código A2A: se expone con el launcher del ADK así:
//
//	go run ./bakery web -port 8001 a2a -a2a_agent_url http://localhost:8001
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
	// Agente de la panadería
	// El Name y la Description se publican en la agent card: es lo que
	// leen los otros agentes para saber qué hace este agente.
	// ----------------------------------------------
	bakeryAgentConfig := llmagent.Config{
		Name:        "bakery_agent",
		Model:       model,
		Description: "Panadería La Espiga: informa los panes disponibles hoy y sus precios.",
		Instruction: `
			Eres el agente de la panadería La Espiga. Atiendes a otros negocios
			que quieren saber qué panes hay hoy y cuánto cuestan.
			Los panes de hoy, con su precio en pesos colombianos, son:
			- Pan de queso: 2.500
			- Almojábana: 3.000
			- Croissant: 4.500
			- Pan integral: 6.000
			Si preguntan por un pan que no está en la lista, di que hoy no lo hay.
		`,
	}
	bakeryAgent, err := llmagent.New(bakeryAgentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Con el sublauncher "a2a", el launcher publica la agent card en
	// /.well-known/agent-card.json y atiende las peticiones A2A.
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(bakeryAgent),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
