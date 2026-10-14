package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagents/parallelagent"
	"google.golang.org/adk/v2/agent/workflowagents/sequentialagent"
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
	// Especialista 1: alérgenos
	// Cada especialista guarda su resultado en una clave distinta del estado.
	// ----------------------------------------------
	allergenCheckerConfig := llmagent.Config{
		Name:        "allergen_checker",
		Model:       model,
		Description: "Identifica los alérgenos de una bebida.",
		Instruction: `
			Eres el responsable de seguridad alimentaria de una cafetería. Lista, en
			máximo 3 viñetas, los alérgenos de la bebida que describe el usuario.
		`,
		OutputKey: "allergens",
	}
	allergenChecker, err := llmagent.New(allergenCheckerConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Especialista 2: precio
	// ----------------------------------------------
	priceEstimatorConfig := llmagent.Config{
		Name:        "price_estimator",
		Model:       model,
		Description: "Estima el costo y el precio de venta de una bebida.",
		Instruction: `
			Eres el contador de una cafetería. Estima en pesos colombianos el costo de los
			ingredientes de la bebida que describe el usuario y sugiere un precio de
			venta. Responde en máximo 2 líneas.
		`,
		OutputKey: "price",
	}
	priceEstimator, err := llmagent.New(priceEstimatorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Especialista 3: marketing
	// ----------------------------------------------
	sloganWriterConfig := llmagent.Config{
		Name:        "slogan_writer",
		Model:       model,
		Description: "Propone el nombre comercial y el eslogan de una bebida.",
		Instruction: `
			Eres el encargado de marketing de una cafetería. Propón un nombre
			comercial y un eslogan corto para la bebida que describe el usuario.
		`,
		OutputKey: "slogan",
	}
	sloganWriter, err := llmagent.New(sloganWriterConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Workflow paralelo
	// Ejecuta los tres especialistas al mismo tiempo. Cada uno trabaja aislado
	// y no ve lo que hacen los otros mientras corre.
	// ----------------------------------------------
	drinkAnalysisAgentConfig := agent.Config{
		Name:        "drink_analysis",
		Description: "Analiza una bebida nueva desde tres áreas al mismo tiempo.",
		SubAgents: []agent.Agent{
			allergenChecker,
			priceEstimator,
			sloganWriter,
		},
	}
	drinkAnalysisConfig := parallelagent.Config{AgentConfig: drinkAnalysisAgentConfig}
	drinkAnalysis, err := parallelagent.New(drinkAnalysisConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente que une los resultados
	// Lee las tres claves que dejaron los especialistas en el estado.
	// ----------------------------------------------
	launchSheetWriterConfig := llmagent.Config{
		Name:        "launch_sheet_writer",
		Model:       model,
		Description: "Arma la ficha de lanzamiento de una bebida nueva.",
		Instruction: `
			Arma en Markdown la ficha de lanzamiento de la bebida nueva, con una
			sección para cada área:

			- Marketing: {slogan}
			- Precio: {price}
			- Alérgenos: {allergens}
		`,
	}
	launchSheetWriter, err := llmagent.New(launchSheetWriterConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Workflow secuencial que envuelve al paralelo
	// Primero el análisis en paralelo y, cuando terminan los tres, la ficha.
	// ----------------------------------------------
	launchPipelineAgentConfig := agent.Config{
		Name:        "launch_pipeline",
		Description: "Analiza una bebida nueva y arma su ficha de lanzamiento.",
		SubAgents: []agent.Agent{
			drinkAnalysis,
			launchSheetWriter,
		},
	}
	launchPipelineConfig := sequentialagent.Config{AgentConfig: launchPipelineAgentConfig}
	launchPipeline, err := sequentialagent.New(launchPipelineConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(launchPipeline),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
