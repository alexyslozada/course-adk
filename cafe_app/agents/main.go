// Sistema de agentes de la cafetería (ADK): un coordinador que delega los pedidos en el barista.
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
	"google.golang.org/genai"
)

// Config agrupa lo que se lee del entorno.
type Config struct {
	GoogleAPIKey string
	MCPURL       string
	MCPToken     string
}

func loadConfig() (Config, error) {
	config := Config{
		GoogleAPIKey: os.Getenv("GOOGLE_API_KEY"),
		MCPURL:       os.Getenv("CAFE_MCP_URL"),
		MCPToken:     os.Getenv("CAFE_MCP_TOKEN"),
	}
	if config.MCPURL == "" {
		config.MCPURL = "http://localhost:8090"
	}
	if config.GoogleAPIKey == "" {
		return config, fmt.Errorf("debes configurar GOOGLE_API_KEY")
	}
	if config.MCPToken == "" {
		return config, fmt.Errorf("debes configurar CAFE_MCP_TOKEN (el mismo del servidor MCP)")
	}
	return config, nil
}

func main() {
	ctx := context.Background()

	config, err := loadConfig()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del modelo
	// ----------------------------------------------
	modelName := "gemini-3.5-flash-lite"

	clientConfig := genai.ClientConfig{
		APIKey: config.GoogleAPIKey,
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// MCP de la cafetería (con token)
	// ----------------------------------------------
	cafeTools, err := cafeMCP(config.MCPURL, config.MCPToken)
	if err != nil {
		fmt.Println("No se pudo cargar el MCP de la cafetería:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Barista en modo task
	// Conversa con el cliente hasta tener el pedido completo, lo registra con
	// create_sale y devuelve su OutputSchema al coordinador con finish_task.
	// ----------------------------------------------
	baristaConfig := llmagent.Config{
		Name:        "barista",
		Model:       model,
		Description: "Toma el pedido de un cliente de la cafetería y lo registra.",
		Instruction: `
			Eres el barista de la cafetería. Tomas el pedido del cliente y lo
			registras. Habla siempre en español, de forma amable y breve, en texto
			plano (sin Markdown: nada de asteriscos ni listas con guiones).

			Pasos:
			1. Consulta el menú con "list_menu". Nunca inventes productos ni precios.
			2. Pregunta lo que falte: qué productos y cuántas unidades. Si pide algo
			   que no está en el menú, dilo y ofrece alternativas del menú.
			3. Pregúntale su nombre. Si está en "list_customers", asócialo a la
			   venta con su id; si no está, registra la venta sin cliente.
			4. Antes de registrar, repite el pedido con el total en pesos y pide
			   confirmación. No uses "create_sale" sin que el cliente confirme.
			5. Registra la venta con "create_sale". Si falla por falta de stock, díselo
			   al cliente (puedes ver el stock con "get_stock") y ofrece cambiar el pedido.
			6. Cuando la venta esté registrada, termina la tarea con el número de la
			   venta, el total y el resumen.
		`,
		Mode:         llmagent.ModeTask,
		OutputSchema: &orderSchema,
		Toolsets:     []tool.Toolset{cafeTools},
	}
	barista, err := llmagent.New(baristaConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Coordinador
	// El barista (ModeTask) se vuelve una tool del coordinador.
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "cafe_coordinator",
		Model:       model,
		Description: "Recibe a los clientes de la cafetería y los atiende.",
		Instruction: `
			Eres el anfitrión de una cafetería. Habla siempre en español, de forma
			amable y breve, en texto plano (sin Markdown: nada de asteriscos ni
			listas con guiones).

			- Si el cliente quiere pedir algo (bebidas, comida, postres), o pregunta
			  qué hay en el menú, usa "barista". Cuando termine, confirma el pedido:
			  número de venta, total en pesos y resumen.
			- Si te saluda, salúdalo y pregúntale en qué le puedes ayudar.
			- Si te pide algo que no tiene que ver con la cafetería, dile con
			  amabilidad que solo puedes ayudar con la cafetería.
		`,
		SubAgents: []agent.Agent{barista},
	}
	coordinator, err := llmagent.New(coordinatorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Se lanza solo el coordinador: el barista vive dentro de él.
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
