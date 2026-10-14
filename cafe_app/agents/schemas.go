package main

import "google.golang.org/genai"

// orderSchema es lo que el barista entrega al coordinador cuando termina su tarea.
var orderSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"order_id": {
			Type:        genai.TypeInteger,
			Description: "Número de la venta creada con create_sale",
		},
		"total": {
			Type:        genai.TypeInteger,
			Description: "Total del pedido en pesos",
		},
		"summary": {
			Type:        genai.TypeString,
			Description: "Resumen breve del pedido: cantidades y productos",
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
