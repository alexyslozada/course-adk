package main

import (
	"fmt"
	"strings"
	"time"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
)

// Route decide el camino del grafo según la clasificación del LLM.
// Es un nodo de código Go: recibe como input la salida del review_classifier
// y emite un evento con la ruta que deben seguir las aristas.
// Su salida (la reseña original) es el input del siguiente nodo.
func Route(ctx agent.Context, input any, emit func(*session.Event) error) (string, error) {
	sentiment := strings.ToLower(strings.TrimSpace(fmt.Sprint(input)))

	event := session.NewEvent(ctx, ctx.InvocationID())
	event.Routes = []string{sentiment}
	err := emit(event)
	if err != nil {
		return "", err
	}

	return userMessage(ctx), nil
}

// CreateTicket registra la queja y genera un número de ticket.
// Es código Go, sin LLM: en una app real guardaría la queja en una base de datos.
func CreateTicket(ctx agent.Context, review string) (string, error) {
	ticket := fmt.Sprintf("TCK-%d", time.Now().Unix()%10000)

	return fmt.Sprintf("Número de ticket: %s\nReseña del cliente: %s", ticket, review), nil
}

// ImprovementQuestion responde a las reseñas neutrales con un texto fijo.
// Demuestra que un nodo final no necesita un LLM.
func ImprovementQuestion(ctx agent.Context, review string) (string, error) {
	return "Gracias por tu opinión. ¿Qué podríamos hacer para que tu próxima visita sea excelente?", nil
}

// userMessage devuelve el texto que escribió el usuario.
func userMessage(ctx agent.Context) string {
	content := ctx.UserContent()
	if content == nil {
		return ""
	}

	var text strings.Builder
	for _, part := range content.Parts {
		text.WriteString(part.Text)
	}

	return strings.TrimSpace(text.String())
}
