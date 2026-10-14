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
	// Sub-agente: barista
	// El coordinador decide a quién delegar leyendo la Description de cada
	// sub-agente, por eso debe ser clara.
	// ----------------------------------------------
	baristaConfig := llmagent.Config{
		Name:        "barista",
		Model:       model,
		Description: "Toma los pedidos de bebidas y recomienda cafés.",
		Instruction: `
			Eres el barista de una cafetería. Ayuda al cliente a elegir y pedir
			su bebida. Cuando el pedido esté listo, confírmalo.
		`,
	}
	barista, err := llmagent.New(baristaConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Sub-agente: atención de reseñas
	// ----------------------------------------------
	reviewAgentConfig := llmagent.Config{
		Name:        "review_agent",
		Model:       model,
		Description: "Atiende las opiniones, quejas y felicitaciones de los clientes.",
		Instruction: `
			Eres el encargado de atención al cliente de una cafetería. Escucha la
			opinión del cliente, agradécele y, si es una queja, discúlpate y
			ofrécele una solución.
		`,
	}
	reviewAgent, err := llmagent.New(reviewAgentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Por ahora los agentes van sueltos: en la web eliges con cuál hablar.
	// ----------------------------------------------
	loader, err := agent.NewMultiLoader(barista, reviewAgent)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	launcherConfig := launcher.Config{
		AgentLoader: loader,
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
