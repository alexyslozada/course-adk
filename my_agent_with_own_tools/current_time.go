package main

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/adk/v2/agent"
)

type CurrentTimeRequest struct {
	City string `json:"city" jsonschema:"Nombre de la ciudad, por ejemplo: Bogotá, Lima, Madrid"`
}

type CurrentTimeResponse struct {
	City string `json:"city"`
	Time string `json:"time"`
}

var cityZones = map[string]string{
	"bogotá":           "America/Bogota",
	"lima":             "America/Lima",
	"ciudad de méxico": "America/Mexico_City",
	"madrid":           "Europe/Madrid",
}

func CurrentTime(ctx agent.Context, req CurrentTimeRequest) (CurrentTimeResponse, error) {
	zone, isFound := cityZones[strings.ToLower(req.City)]
	if !isFound {
		return CurrentTimeResponse{}, fmt.Errorf("no conozco la zona horaria de: %s", req.City)
	}

	loc, err := time.LoadLocation(zone)
	if err != nil {
		return CurrentTimeResponse{}, fmt.Errorf("no pude generar la zona horaria de %s", req.City)
	}

	return CurrentTimeResponse{
		City: req.City,
		Time: time.Now().In(loc).Format("15:04"),
	}, nil
}
