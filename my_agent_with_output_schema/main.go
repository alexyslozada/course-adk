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
	// Creación del agente con salida estructurada
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "review_agent",
		Model:       model,
		Description: "Clasificas las reseñas que dejan los clientes de una cafetería",
		Instruction: `
			Eres el analista de reseñas de una cafetería. El usuario te enviará la
			reseña de un cliente y debes clasificarla.
		`,
		// El modelo siempre responde un JSON que cumple este esquema
		OutputSchema: &reviewSchema,
		// La respuesta del agente se guarda en el estado de la sesión con esta
		// clave. Otro agente o tool podría leerla después como {review}.
		OutputKey: "review",
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
