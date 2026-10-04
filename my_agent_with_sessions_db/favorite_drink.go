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

// Con el prefijo "user:" el dato se comparte entre todas las sesiones del usuario.
// Sin prefijo, solo existiría en la sesión actual.
const favoriteDrinkKey = "user:favorite_drink"

func SaveFavoriteDrink(ctx agent.Context, req SaveFavoriteDrinkRequest) (SaveFavoriteDrinkResponse, error) {
	err := ctx.State().Set(favoriteDrinkKey, req.Drink)
	if err != nil {
		return SaveFavoriteDrinkResponse{}, err
	}

	return SaveFavoriteDrinkResponse{Status: "Bebida favorita guardada: " + req.Drink}, nil
}
