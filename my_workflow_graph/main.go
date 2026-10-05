package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/workflow"
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
	// Nodos del grafo
	// AgentNode envuelve un agente (LLM). FunctionNode envuelve una función Go.
	// La salida de cada nodo es el input del siguiente.
	// ----------------------------------------------
	classifierNode, err := workflow.NewAgentNode(reviewClassifier, workflow.NodeConfig{})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	thanksNode, err := workflow.NewAgentNode(thanksWriter, workflow.NodeConfig{})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	apologyNode, err := workflow.NewAgentNode(apologyWriter, workflow.NodeConfig{})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// "Emitting" porque además de devolver su salida, emite el evento con la ruta
	routerNode := workflow.NewEmittingFunctionNode("router", Route, workflow.NodeConfig{})
	ticketNode := workflow.NewFunctionNode("create_ticket", CreateTicket, workflow.NodeConfig{})
	improvementNode := workflow.NewFunctionNode("improvement_question", ImprovementQuestion, workflow.NodeConfig{})

	// ----------------------------------------------
	// Aristas del grafo
	// Chain une los nodos en línea. Las aristas con Route solo se siguen si el
	// router emitió esa ruta. workflow.Default se sigue si ninguna otra coincide.
	// ----------------------------------------------
	edges := workflow.Concat(
		workflow.Chain(workflow.Start, classifierNode, routerNode),
		[]workflow.Edge{
			{From: routerNode, To: thanksNode, Route: workflow.StringRoute("positivo")},
			{From: routerNode, To: ticketNode, Route: workflow.StringRoute("negativo")},
			{From: routerNode, To: improvementNode, Route: workflow.Default},
		},
		workflow.Chain(ticketNode, apologyNode),
	)

	// ----------------------------------------------
	// Workflow de grafo
	// ----------------------------------------------
	reviewGraphConfig := workflowagent.Config{
		Name:        "review_graph",
		Description: "Clasifica una reseña y la responde por el camino adecuado.",
		Edges:       edges,
		SubAgents: []agent.Agent{
			reviewClassifier,
			thanksWriter,
			apologyWriter,
		},
	}
	reviewGraph, err := workflowagent.New(reviewGraphConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(reviewGraph),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
