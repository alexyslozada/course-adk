package main

import (
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
)

// Precios reales del menú, en pesos colombianos. El combo_planner conoce los productos, pero no
// estos precios: solo el cajero los ve a través de esta tool.
var menuPrices = map[string]float64{
	"café americano":     4500,
	"capuchino":          7000,
	"latte":              8000,
	"chocolate caliente": 6500,
	"té":                 4000,
	"jugo de naranja":    6000,
	"croissant":          5000,
	"pan de queso":       3500,
	"empanada":           3000,
	"sándwich de pollo":  14000,
	"torta de chocolate": 9000,
	"galleta":            2500,
}

type ComboItem struct {
	Product  string `json:"product" jsonschema:"Nombre del producto tal como aparece en el menú"`
	Quantity int    `json:"quantity" jsonschema:"Cantidad del producto"`
}

type CheckComboRequest struct {
	Items  []ComboItem `json:"items" jsonschema:"Productos del combo"`
	Budget float64     `json:"budget" jsonschema:"Presupuesto máximo del cliente en pesos colombianos"`
}

type ComboLine struct {
	Product   string  `json:"product"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Subtotal  float64 `json:"subtotal"`
}

type CheckComboResponse struct {
	Lines        []ComboLine `json:"lines"`
	Total        float64     `json:"total"`
	Budget       float64     `json:"budget"`
	WithinBudget bool        `json:"within_budget"`
	// Positivo: cuánto sobra. Negativo: cuánto se pasó del presupuesto.
	Difference float64 `json:"difference"`
}

func CheckCombo(ctx agent.Context, req CheckComboRequest) (CheckComboResponse, error) {
	response := CheckComboResponse{Budget: req.Budget}

	for _, item := range req.Items {
		price, isFound := menuPrices[strings.ToLower(item.Product)]
		if !isFound {
			return CheckComboResponse{}, fmt.Errorf("el producto %s no está en el menú", item.Product)
		}

		subtotal := price * float64(item.Quantity)
		response.Lines = append(response.Lines, ComboLine{
			Product:   item.Product,
			Quantity:  item.Quantity,
			UnitPrice: price,
			Subtotal:  subtotal,
		})
		response.Total += subtotal
	}

	response.Difference = req.Budget - response.Total
	response.WithinBudget = response.Total <= req.Budget

	// La condición de salida del bucle la decide el código, no el LLM:
	// si el combo cumple el presupuesto, Escalate detiene el loopagent
	// cuando el cajero termine de responder.
	if response.WithinBudget {
		ctx.Actions().Escalate = true
	}

	return response, nil
}
