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
	// Agentes (LLM)
	// ----------------------------------------------
	reviewClassifierConfig := llmagent.Config{
		Name:        "review_classifier",
		Model:       model,
		Description: "Clasifica el sentimiento de una reseña.",
		Instruction: `
			Clasifica el sentimiento de la reseña de una cafetería que envía el
			usuario. Responde exactamente una palabra, en minúscula y sin puntuación:
			positivo, negativo o neutral.
		`,
	}
	reviewClassifier, err := llmagent.New(reviewClassifierConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	thanksWriterConfig := llmagent.Config{
		Name:        "thanks_writer",
		Model:       model,
		Description: "Agradece una reseña positiva.",
		Instruction: `
			Eres el encargado de atención al cliente de una cafetería. Agradece de
			forma breve y cálida la reseña positiva que recibes.
		`,
	}
	thanksWriter, err := llmagent.New(thanksWriterConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	apologyWriterConfig := llmagent.Config{
		Name:        "apology_writer",
		Model:       model,
		Description: "Se disculpa por una reseña negativa.",
		Instruction: `
			Eres el encargado de atención al cliente de una cafetería. Recibes una
			queja con su número de ticket. Discúlpate de forma breve, menciona el
			número de ticket y explica que lo contactaremos para solucionarlo.
		`,
	}
	apologyWriter, err := llmagent.New(apologyWriterConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Por ahora los agentes van sueltos: en la web eliges con cuál hablar.
	// ----------------------------------------------
	loader, err := agent.NewMultiLoader(reviewClassifier, thanksWriter, apologyWriter)
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
