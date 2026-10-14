package main

import "google.golang.org/genai"

var sentiment = genai.Schema{
	Type:        genai.TypeString,
	Description: "Sentimiento general de la reseña",
	Enum:        []string{"positivo", "negativo", "neutral"},
}

var rating = genai.Schema{
	Type:        genai.TypeInteger,
	Description: "Calificación estimada de 1 a 5",
	Minimum:     new(1.0),
	Maximum:     new(5.0),
}

var summary = genai.Schema{
	Type:        genai.TypeString,
	Description: "Resumen de la reseña en una sola frase",
}

// reviewSchema define la forma exacta del JSON que debe responder el agente.
// En Go el esquema se escribe a mano con genai.Schema.
var reviewSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"sentiment": &sentiment,
		"rating":    &rating,
		"summary":   &summary,
	},
	Required: []string{"sentiment", "rating", "summary"},
}
