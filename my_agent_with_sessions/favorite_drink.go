package main

import (
	"google.golang.org/adk/v2/agent"
)

type SaveFavoriteDrinkRequest struct {
	Drink string `json:"drink" jsonschema:"Bebida favorita del cliente, por ejemplo: latte con leche de avena"`
}

type SaveFavoriteDrinkResponse struct {
	Status string `json:"status"`
}

func SaveFavoriteDrink(ctx agent.Context, req SaveFavoriteDrinkRequest) (SaveFavoriteDrinkResponse, error) {
	return SaveFavoriteDrinkResponse{Status: "Bebida favorita guardada: " + req.Drink}, nil
}
