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
	// Agente que arma el combo
	// Conoce los productos del menú, pero no sus precios: solo se entera de
	// ellos por las correcciones del cajero. En la primera vuelta {feedback?}
	// está vacío.
	// ----------------------------------------------
	comboPlannerConfig := llmagent.Config{
		Name:        "combo_planner",
		Model:       model,
		Description: "Arma un combo con los productos del menú.",
		Instruction: `
			Eres el encargado de armar combos en una cafetería. Arma un combo para lo
			que pide el usuario, usando solo productos de este menú:
			café americano, capuchino, latte, chocolate caliente, té, jugo de naranja,
			croissant, pan de queso, empanada, sándwich de pollo, torta de chocolate,
			galleta.

			Correcciones del cajero sobre tu combo anterior: {feedback?}

			Si hay correcciones, ajusta el combo para cumplir el presupuesto: cambia
			productos por otros más baratos o reduce cantidades, sin dejar a nadie
			sin bebida.
			Responde solo con la lista del combo, una línea por producto con su
			cantidad.
		`,
		OutputKey: "combo",
	}
	comboPlanner, err := llmagent.New(comboPlannerConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Tool que calcula el total con los precios reales
	// Si el combo cumple el presupuesto, ella misma detiene el bucle.
	// ----------------------------------------------
	checkComboToolConfig := functiontool.Config{
		Name:        "check_combo",
		Description: "Calcula el total de un combo con los precios del menú y verifica si cumple el presupuesto.",
	}
	checkComboTool, err := functiontool.New(checkComboToolConfig, CheckCombo)
	if err != nil {
		fmt.Printf("Hubo un error al convertir la función %s a tool", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente cajero
	// No opina: verifica el combo con la tool y reporta el resultado.
	// ----------------------------------------------
	cashierConfig := llmagent.Config{
		Name:        "cashier",
		Model:       model,
		Description: "Verifica que el combo cumpla el presupuesto del cliente.",
		Instruction: `
			Eres el cajero de una cafetería. Verifica este combo:

			{combo}

			Usa siempre la herramienta "check_combo" con los productos del combo y el
			presupuesto que indicó el usuario. Los montos están en pesos colombianos:
			muéstralos con el formato $50.000.
			- Si within_budget es true, responde "Combo aprobado" con la lista de
			  productos, sus precios y el total.
			- Si within_budget es false, responde cuánto se pasó del presupuesto y el
			  precio de cada producto del combo, para que lo puedan ajustar.
		`,
		Tools:     []tool.Tool{checkComboTool},
		OutputKey: "feedback",
	}
	cashier, err := llmagent.New(cashierConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Por ahora los agentes van sueltos: en la web eliges con cuál hablar.
	// ----------------------------------------------
	loader, err := agent.NewMultiLoader(comboPlanner, cashier)
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
