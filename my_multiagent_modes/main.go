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
	// Sub-agente en modo task
	// Conversa con el usuario hasta completar la tarea. El ADK le agrega la
	// tool finish_task: al llamarla devuelve su OutputSchema y el control al
	// coordinador.
	// ----------------------------------------------
	loyaltySignupConfig := llmagent.Config{
		Name:        "loyalty_signup",
		Model:       model,
		Description: "Inscribe a un cliente en el club de la cafetería.",
		Instruction: `
			Inscribe al cliente en el club de la cafetería. Pregúntale, uno por uno,
			los datos que falten: su nombre, su bebida favorita y su mes de
			cumpleaños.
		`,
		Mode:         llmagent.ModeTask,
		OutputSchema: &loyaltySchema,
		// Una tarea no puede transferir la conversación: debe terminar con finish_task.
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}
	loyaltySignup, err := llmagent.New(loyaltySignupConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Sub-agente en modo single_turn
	// No habla con el usuario: recibe su InputSchema, responde su OutputSchema
	// en un solo turno y devuelve el control al coordinador.
	// ----------------------------------------------
	reviewClassifierConfig := llmagent.Config{
		Name:         "review_classifier",
		Model:        model,
		Description:  "Clasifica la reseña de un cliente.",
		Instruction:  "Eres el analista de reseñas de una cafetería. Clasifica la reseña que recibes.",
		Mode:         llmagent.ModeSingleTurn,
		InputSchema:  &reviewInputSchema,
		OutputSchema: &reviewSchema,
		// Sin esto el ADK le deja transfer_to_agent hacia el coordinador y el
		// modelo puede devolverle la conversación en vez de responder el JSON.
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}
	reviewClassifier, err := llmagent.New(reviewClassifierConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente coordinador
	// Los sub-agentes en modo task y single_turn no se alcanzan con
	// transfer_to_agent: el ADK los convierte en tools del coordinador.
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "coffee_host",
		Model:       model,
		Description: "Recibe a los clientes de la cafetería y los atiende.",
		Instruction: `
			Eres el anfitrión de una cafetería.
			- Si el cliente quiere unirse al club, usa "loyalty_signup". Cuando
			  termine, dale la bienvenida al club por su nombre.
			- Si el cliente deja una opinión, clasifícala con "review_classifier" y
			  respóndele según el sentimiento: agradece si es positiva y
			  discúlpate si es negativa.
		`,
		SubAgents: []agent.Agent{
			loyaltySignup,
			reviewClassifier,
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
