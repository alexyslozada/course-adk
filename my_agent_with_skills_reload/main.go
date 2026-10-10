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
	"google.golang.org/adk/v2/tool/skilltoolset"
	"google.golang.org/adk/v2/tool/skilltoolset/skill"
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
	// Skills desde el sistema de archivos, sin caché
	// NewFileSystemSource vuelve a leer ./skills cada vez que el agente
	// le pide la lista, y el agente la pide antes de cada llamada al modelo.
	// Por eso una carpeta nueva queda disponible en el siguiente mensaje,
	// sin reiniciar el programa.
	// ----------------------------------------------
	source := skill.NewFileSystemSource(os.DirFS("./skills"))

	skillToolset, err := skilltoolset.New(ctx, skilltoolset.Config{
		Source: source,
	})
	if err != nil {
		fmt.Println("Error al crear el skill toolset:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del agente
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "cafe_assistant",
		Model:       model,
		Description: "Asistente de la cafetería que usa skills",
		Instruction: `Eres el asistente de una cafetería. Respondes en español y de forma breve.
Usa las skills disponibles cuando la pregunta tenga que ver con ellas.
Si ninguna skill cubre la pregunta, dilo con sinceridad y no inventes datos de la cafetería.`,
		Toolsets: []tool.Toolset{skillToolset},
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
