package main

import "google.golang.org/genai"

// orderSchema es lo que el barista entrega al coordinador cuando termina su tarea.
var orderSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"order_id": {
			Type:        genai.TypeInteger,
			Description: "Número de la venta registrada; 0 si el cliente canceló el pedido",
		},
		"total": {
			Type:        genai.TypeInteger,
			Description: "Total del pedido en pesos",
		},
		"summary": {
			Type:        genai.TypeString,
			Description: "Resumen breve del pedido: cantidades y productos",
		},
		"pending_request": {
			Type:        genai.TypeString,
			Description: "Si el cliente dejó el pedido para pedir otra cosa, lo que pidió; vacío si no",
		},
	},
	Required: []string{"order_id", "total", "summary"},
}

// adminSchema es lo que el administrador entrega al coordinador cuando termina.
var adminSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"status": {
			Type:        genai.TypeString,
			Description: "Resultado: hecho si se aplicó el cambio, rechazado si el administrador no lo aprobó, sin_cambios si no hubo nada que cambiar",
			Enum:        []string{"hecho", "rechazado", "sin_cambios"},
		},
		"summary": {
			Type:        genai.TypeString,
			Description: "Qué se hizo, en una frase",
		},
	},
	Required: []string{"status", "summary"},
}

// reportSchema es la forma del reporte del día que entrega el redactor.
var reportSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"headline": {
			Type:        genai.TypeString,
			Description: "Titular del día en una frase",
		},
		"sales_summary": {
			Type:        genai.TypeString,
			Description: "Resumen de las ventas: cantidad, total en pesos y productos más vendidos",
		},
		"stock_alerts": {
			Type:        genai.TypeArray,
			Description: "Una alerta por cada producto en o bajo su mínimo; vacía si no hay",
			Items:       &genai.Schema{Type: genai.TypeString},
		},
		"review_summary": {
			Type:        genai.TypeString,
			Description: "Resumen de las reseñas: cantidad, promedio y las negativas",
		},
		"recommendations": {
			Type:        genai.TypeArray,
			Description: "Entre 1 y 3 recomendaciones concretas",
			Items:       &genai.Schema{Type: genai.TypeString},
		},
	},
	Required: []string{"headline", "sales_summary", "stock_alerts", "review_summary", "recommendations"},
}

// comboParamsSchema son los datos que se extraen de la conversación para armar un combo.
// Un valor que el cliente no ha dado se deja en 0 (o vacío): el código decide qué hacer.
var comboParamsSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"people": {
			Type:        genai.TypeInteger,
			Description: "Número de personas para las que es el combo; 0 si no lo ha dicho",
		},
		"budget": {
			Type:        genai.TypeInteger,
			Description: "Presupuesto máximo en pesos, como número entero (por ejemplo 48000); 0 si no lo ha dicho",
		},
		"food_categories": {
			Type:        genai.TypeArray,
			Description: "Categorías de alimento que prefiere el cliente; vacío si no dijo nada",
			Items:       &genai.Schema{Type: genai.TypeString, Enum: []string{"Panadería", "Comida", "Postres"}},
		},
		"notes": {
			Type:        genai.TypeString,
			Description: "Preferencias del cliente para el combo (por ejemplo: carbohidratos, sin dulce); vacío si no hay",
		},
	},
	Required: []string{"people", "budget", "food_categories", "notes"},
}

// reviewTaskSchema es lo que el agente de reseñas entrega al coordinador cuando termina.
var reviewTaskSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"status": {
			Type:        genai.TypeString,
			Description: "Resultado: guardada si la reseña quedó guardada, cancelada si el cliente no la aprobó o desistió",
			Enum:        []string{"guardada", "cancelada"},
		},
		"summary": {
			Type:        genai.TypeString,
			Description: "Qué pasó, en una frase",
		},
	},
	Required: []string{"status", "summary"},
}
