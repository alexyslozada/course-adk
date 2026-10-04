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

// ----------------------------------------------
// Entrada del review_classifier (single_turn)
// Son los parámetros que el coordinador debe enviar al llamarlo.
// ----------------------------------------------
var review = genai.Schema{
	Type:        genai.TypeString,
	Description: "Texto de la reseña que dejó el cliente",
}

var reviewInputSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"review": &review,
	},
	Required: []string{"review"},
}

// ----------------------------------------------
// Salida del loyalty_signup (task)
// Es el resultado que devuelve al coordinador al terminar la tarea.
// ----------------------------------------------
var name = genai.Schema{
	Type:        genai.TypeString,
	Description: "Nombre del cliente",
}

var favoriteDrink = genai.Schema{
	Type:        genai.TypeString,
	Description: "Bebida favorita del cliente",
}

var birthdayMonth = genai.Schema{
	Type:        genai.TypeString,
	Description: "Mes de cumpleaños del cliente",
}

var loyaltySchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"name":           &name,
		"favorite_drink": &favoriteDrink,
		"birthday_month": &birthdayMonth,
	},
	Required: []string{"name", "favorite_drink", "birthday_month"},
}
