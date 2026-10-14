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
	"google.golang.org/adk/v2/tool/functiontool"
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
	// Tool con confirmación humana (Human in the Loop)
	// La confirmación se pide dentro de la función con ctx.RequestConfirmation,
	// así la persona puede modificar el descuento además de aprobar o rechazar.
	// ----------------------------------------------
	applyCouponToolConfig := functiontool.Config{
		Name:        "apply_coupon",
		Description: "Aplica un cupón de descuento a un producto y devuelve el precio final.",
	}
	applyCouponTool, err := functiontool.New(applyCouponToolConfig, ApplyCoupon)
	if err != nil {
		fmt.Printf("Hubo un error al convertir la función %s a tool", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del agente
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "store_agent",
		Model:       model,
		Description: "Ayudas al usuario a aplicar cupones de descuento en la tienda",
		Instruction: `
			Eres el asistente de una tienda. Cuando el usuario quiera aplicar un cupón
			a un producto, usa la herramienta "apply_coupon" y respóndele con el precio
			original, el descuento y el precio final.
			Si el cupón requiere aprobación, avísale al usuario que está pendiente.
			Si la herramienta responde que la llamada fue rechazada, dile al usuario
			que el supervisor rechazó el cupón. Si se aplicó, indica si el supervisor
			modificó el descuento.
		`,
		Tools: []tool.Tool{
			applyCouponTool,
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
