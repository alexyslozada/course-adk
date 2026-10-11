package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

func main() {
	ctx := context.Background()

	// ----------------------------------------------
	// Creación del modelo según el proveedor
	// ----------------------------------------------
	provider := os.Getenv("MODEL_PROVIDER")
	if provider == "" {
		provider = "gemini"
	}

	llm, err := newModel(ctx, provider)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Convertir la función en una function tool
	// ----------------------------------------------
	drinkPriceToolConfig := functiontool.Config{
		Name:        "get_drink_price",
		Description: "Devuelve el precio en pesos colombianos de una bebida del menú.",
	}
	drinkPriceTool, err := functiontool.New(drinkPriceToolConfig, DrinkPrice)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del agente (igual para cualquier proveedor)
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "barista",
		Model:       llm,
		Description: "Barista que responde precios de las bebidas de la cafetería",
		Instruction: `
			Eres el barista de una cafetería. Respondes en español y de forma breve.
			**Regla de precios:** siempre usa la herramienta "get_drink_price" para responder
			el precio de una bebida. Si la herramienta devuelve un error, dile al usuario
			que esa bebida no está en el menú. Nunca inventes precios.
		`,
		Tools: []tool.Tool{
			drinkPriceTool,
		},
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

// newModel crea el model.LLM del proveedor elegido. El agente no sabe
// cuál es: solo recibe algo que cumple la interfaz model.LLM.
func newModel(ctx context.Context, provider string) (model.LLM, error) {
	modelName := os.Getenv("MODEL_NAME")

	switch provider {
	case "gemini":
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("debes configurar GOOGLE_API_KEY")
		}
		if modelName == "" {
			modelName = "gemini-3.8-flash"
		}

		clientConfig := genai.ClientConfig{
			APIKey: apiKey,
		}
		return gemini.NewModel(ctx, modelName, &clientConfig)

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("debes configurar OPENAI_API_KEY")
		}
		if modelName == "" {
			modelName = "gpt-5-mini"
		}

		// Sin API ni BaseURL usa la Responses API de OpenAI
		clientConfig := openaimodel.ClientConfig{
			APIKey: apiKey,
		}
		return openaimodel.NewModel(ctx, modelName, &clientConfig)

	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("debes configurar ANTHROPIC_API_KEY")
		}
		if modelName == "" {
			modelName = "claude-haiku-4-5"
		}

		// Anthropic expone un endpoint compatible con la Chat Completions API de OpenAI
		clientConfig := openaimodel.ClientConfig{
			APIKey:  apiKey,
			BaseURL: "https://api.anthropic.com/v1/",
			API:     openaimodel.APIChatCompletions,
		}
		return openaimodel.NewModel(ctx, modelName, &clientConfig)

	default:
		return nil, fmt.Errorf("proveedor %q no soportado: usa gemini, openai o anthropic", provider)
	}
}
