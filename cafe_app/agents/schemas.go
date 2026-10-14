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
