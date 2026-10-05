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
	orderParserConfig := llmagent.Config{
		Name:        "order_parser",
		Model:       model,
		Description: "Extrae las bebidas del pedido de un cliente.",
		Instruction: `
			Eres el cajero de una cafetería. Extrae las bebidas que pide el cliente.
			Si el mensaje no contiene ningún pedido de bebidas, responde una lista vacía.
		`,
		// El modelo siempre responde un JSON con la lista de bebidas
		OutputSchema: &orderSchema,
	}
	orderParser, err := llmagent.New(orderParserConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	baristaConfig := llmagent.Config{
		Name:        "barista",
		Model:       model,
		Description: "Explica cómo se prepara una bebida.",
		Instruction: `
			Eres el barista de una cafetería. Recibes el nombre de una bebida y
			explicas en máximo dos líneas cómo la preparas.
		`,
	}
	barista, err := llmagent.New(baristaConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Nodos
	// El parser y el barista no se conectan con aristas:
	// el nodo dinámico los llama desde código Go con workflow.RunNode
	// ----------------------------------------------
	parserNode, err := workflow.NewAgentNode(orderParser, workflow.NodeConfig{})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	baristaNode, err := workflow.NewAgentNode(barista, workflow.NodeConfig{})
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	orderWorkflowNode := NewOrderWorkflow(parserNode, baristaNode)

	// ----------------------------------------------
	// Workflow dinámico
	// Solo hay una arista: Start → order_workflow. Todo lo demás
	// ocurre dentro del código Go del nodo dinámico.
	// ----------------------------------------------
	orderAgentConfig := workflowagent.Config{
		Name:        "order_dynamic",
		Description: "Prepara un pedido con cualquier cantidad de bebidas.",
		Edges:       workflow.Chain(workflow.Start, orderWorkflowNode),
		SubAgents: []agent.Agent{
			orderParser,
			barista,
		},
	}
	orderAgent, err := workflowagent.New(orderAgentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(orderAgent),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
