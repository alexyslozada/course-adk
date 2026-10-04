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
	// Agente coordinador
	// Con SubAgents, el ADK le agrega la tool transfer_to_agent y el LLM
	// decide a qué sub-agente pasarle la conversación.
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "coffee_host",
		Model:       model,
		Description: "Recibe a los clientes de la cafetería y los dirige al área correcta.",
		Instruction: `
			Eres el anfitrión de una cafetería. Saluda al cliente y, según lo que
			necesite, pásale la conversación al agente adecuado:
			- Si quiere pedir una bebida, al "barista".
			- Si quiere dar una opinión o poner una queja, al "review_agent".
			No respondas tú mismo esas solicitudes.
		`,
		SubAgents: []agent.Agent{
			barista,
			reviewAgent,
		},
	}
	coordinator, err := llmagent.New(coordinatorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Se lanza solo el coordinador: los sub-agentes viven dentro de él.
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
