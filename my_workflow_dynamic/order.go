package main

import (
	"google.golang.org/genai"
)

var drinks = genai.Schema{
	Type:        genai.TypeArray,
	Description: "Una entrada por cada bebida del pedido. Si pide dos lattes, van dos entradas",
	Items: &genai.Schema{
		Type:        genai.TypeString,
		Description: "Nombre de la bebida, por ejemplo: latte, capuchino, té chai",
	},
}

// orderSchema obliga al order_parser a responder la lista de bebidas en JSON
var orderSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"drinks": &drinks,
	},
	Required: []string{"drinks"},
}

type Order struct {
	Drinks []string `json:"drinks"`
}
