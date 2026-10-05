package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/workflow"
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

// NewOrderWorkflow crea el nodo dinámico que orquesta el pedido.
// No hay aristas entre el parser y el barista: el orden de ejecución
// lo decide este código Go, llamando a cada nodo con workflow.RunNode.
func NewOrderWorkflow(parserNode, baristaNode workflow.Node) workflow.Node {
	orderWorkflow := func(ctx agent.Context, request string, emit func(*session.Event) error) (string, error) {
		// 1. El order_parser (LLM) extrae las bebidas del pedido.
		// Como tiene OutputSchema, el ADK ya convierte su JSON en un map
		parserOutput, err := workflow.RunNode[map[string]any](ctx, parserNode, request)
		if err != nil {
			return "", err
		}

		// Pasamos el map a nuestro struct Order
		jsonBytes, err := json.Marshal(parserOutput)
		if err != nil {
			return "", err
		}
		var order Order
		err = json.Unmarshal(jsonBytes, &order)
		if err != nil {
			return "", fmt.Errorf("el order_parser no devolvió la lista de bebidas: %w", err)
		}

		// 2. Si no hay bebidas, respondemos sin llamar al barista
		if len(order.Drinks) == 0 {
			return "No entendí tu pedido. ¿Qué bebidas quieres?", nil
		}

		// 3. Llamamos al barista una vez por cada bebida.
		// La cantidad de pasos depende del pedido: eso no se puede
		// declarar por adelantado con las aristas de un grafo.
		var ticket strings.Builder
		fmt.Fprintf(&ticket, "Tu pedido tiene %d bebida(s):\n", len(order.Drinks))

		for i, drink := range order.Drinks {
			// WithUseSubBranch: cada llamada del barista solo ve su bebida,
			// no el historial de las llamadas anteriores
			preparation, err := workflow.RunNode[string](ctx, baristaNode, drink, workflow.WithUseSubBranch())
			if err != nil {
				return "", err
			}

			fmt.Fprintf(&ticket, "\n%d. %s\n%s\n", i+1, drink, strings.TrimSpace(preparation))
		}

		// 4. Lo que devuelve el nodo dinámico es la respuesta final del workflow
		return ticket.String(), nil
	}

	return workflow.NewDynamicNode("order_workflow", orderWorkflow, workflow.NodeConfig{})
}
