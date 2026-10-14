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
	// Paso 1: leer la factura
	// El usuario adjunta la factura (PDF o imagen) y el modelo extrae los
	// ítems. El resultado se guarda en el estado con la clave "invoice_items".
	// ----------------------------------------------
	invoiceReaderConfig := llmagent.Config{
		Name:         "invoice_reader",
		Model:        model,
		Description:  "Lee una factura y extrae sus ítems.",
		Instruction:  "Lee la factura que adjuntó el usuario y extrae todos sus ítems.",
		OutputSchema: &invoiceItemsSchema,
		OutputKey:    "invoice_items",
	}
	invoiceReader, err := llmagent.New(invoiceReaderConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Paso 2: agrupar los ítems por categoría
	// {invoice_items} se reemplaza con lo que guardó el paso 1 en el estado.
	// ----------------------------------------------
	itemCategorizerConfig := llmagent.Config{
		Name:        "item_categorizer",
		Model:       model,
		Description: "Agrupa los ítems de una factura por categoría.",
		Instruction: `
			Eres el contador de una cafetería. Agrupa por categoría los ítems de la
			factura y calcula el subtotal de cada categoría.

			Los ítems de la factura son: {invoice_items}
		`,
		OutputSchema: &categoriesSchema,
		OutputKey:    "categories",
	}
	itemCategorizer, err := llmagent.New(itemCategorizerConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Paso 3: presentar el reporte
	// Sin OutputSchema: responde texto libre (Markdown) para que lo lea una
	// persona. {categories} se reemplaza con lo que guardó el paso 2.
	// ----------------------------------------------
	reportWriterConfig := llmagent.Config{
		Name:        "report_writer",
		Model:       model,
		Description: "Presenta el reporte de gastos de una factura.",
		Instruction: `
			Presenta el reporte de gastos de la factura en Markdown:
			1. Una tabla con las columnas Categoría, Ítems y Subtotal, y una fila
			   final con el total.
			2. Una frase indicando en qué categoría se gastó más.

			Los ítems agrupados por categoría son: {categories}
		`,
	}
	reportWriter, err := llmagent.New(reportWriterConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Por ahora los agentes van sueltos: en la web eliges con cuál hablar.
	// ----------------------------------------------
	loader, err := agent.NewMultiLoader(invoiceReader, itemCategorizer, reportWriter)
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
