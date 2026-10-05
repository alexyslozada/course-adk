package main

import (
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
)

type DrinkPriceInput struct {
	Drink string `json:"drink" jsonschema:"Nombre de la bebida, por ejemplo: capuchino"`
}

type DrinkPriceOutput struct {
	Drink string `json:"drink"`
	Price int    `json:"price"`
}

// menu tiene los precios en pesos colombianos.
var menu = map[string]int{
	"americano": 6500,
	"capuchino": 8500,
	"latte":     9000,
	"mocaccino": 10500,
}

func DrinkPrice(_ agent.Context, input DrinkPriceInput) (DrinkPriceOutput, error) {
	drink := strings.ToLower(strings.TrimSpace(input.Drink))

	price, ok := menu[drink]
	if !ok {
		return DrinkPriceOutput{}, fmt.Errorf("la bebida %q no está en el menú", input.Drink)
	}

	return DrinkPriceOutput{Drink: drink, Price: price}, nil
}
