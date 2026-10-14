// Panadería La Espiga: otro negocio, con su propio agente, que la cafetería usa por A2A.
// No tiene código A2A: se expone con el launcher del ADK así:
//
//	go run . web -port 8001 a2a -a2a_agent_url http://localhost:8001
package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
)

// catalog son los panes de la panadería y su precio por unidad para negocios, en pesos.
var catalog = map[string]int{
	"Pan de queso": 2000,
	"Croissant":    3500,
	"Almojábana":   2500,
	"Pan integral": 6000,
}

// maxQuantity es lo máximo que la panadería despacha de un pan en un pedido.
const maxQuantity = 200

// ----------------------------------------------
// place_order: el código valida el pan y la cantidad, calcula el total y numera el pedido
// ----------------------------------------------

type PlaceOrderRequest struct {
	Product  string `json:"product" jsonschema:"nombre del pan, tal como aparece en el catálogo"`
	Quantity int    `json:"quantity" jsonschema:"unidades que pide el cliente"`
}

type PlaceOrderResponse struct {
	Status   string `json:"status"`
	OrderID  int    `json:"order_id,omitempty"`
	Product  string `json:"product,omitempty"`
	Quantity int    `json:"quantity,omitempty"`
	Total    int    `json:"total,omitempty"`
	Message  string `json:"message"`
}

// orders guarda los pedidos en memoria: basta para el ejemplo (se pierden al reiniciar).
type orders struct {
	mu     sync.Mutex
	lastID int
}

func (o *orders) PlaceOrder(_ agent.Context, req PlaceOrderRequest) (PlaceOrderResponse, error) {
	name, price, ok := findBread(req.Product)
	if !ok {
		return PlaceOrderResponse{Status: "rechazado", Message: fmt.Sprintf("No horneamos %q. Panes disponibles: %s.", req.Product, breadNames())}, nil
	}
	if req.Quantity < 1 || req.Quantity > maxQuantity {
		return PlaceOrderResponse{Status: "rechazado", Message: fmt.Sprintf("La cantidad debe estar entre 1 y %d unidades.", maxQuantity)}, nil
	}

	o.mu.Lock()
	o.lastID++
	id := 1000 + o.lastID
	o.mu.Unlock()

	return PlaceOrderResponse{
		Status:   "confirmado",
		OrderID:  id,
		Product:  name,
		Quantity: req.Quantity,
		Total:    req.Quantity * price,
		Message:  "Pedido confirmado. Se entrega hoy.",
	}, nil
}

// findBread busca un pan sin importar mayúsculas ni espacios de más.
func findBread(product string) (string, int, bool) {
	for name, price := range catalog {
		if strings.EqualFold(name, strings.TrimSpace(product)) {
			return name, price, true
		}
	}
	return "", 0, false
}

func breadNames() string {
	return strings.Join(slices.Sorted(maps.Keys(catalog)), ", ")
}

// catalogText es el catálogo para la instrucción, generado desde el mismo mapa que usa place_order.
func catalogText() string {
	var lines []string
	for _, name := range slices.Sorted(maps.Keys(catalog)) {
		lines = append(lines, fmt.Sprintf("- %s: %d pesos la unidad", name, catalog[name]))
	}
	return strings.Join(lines, "\n")
}

// orderSchema es lo que la panadería le responde a quien le hace un pedido: un contrato, no texto libre.
var orderSchema = genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"status": {
			Type:        genai.TypeString,
			Description: "confirmado si se tomó el pedido, rechazado si no, info si solo fue una consulta",
			Enum:        []string{"confirmado", "rechazado", "info"},
		},
		"order_id": {
			Type:        genai.TypeInteger,
			Description: "Número del pedido que devolvió place_order; 0 si no hay pedido",
		},
		"product": {
			Type:        genai.TypeString,
			Description: "Pan del pedido, tal como lo devolvió place_order",
		},
		"quantity": {
			Type:        genai.TypeInteger,
			Description: "Unidades del pedido, tal como las devolvió place_order",
		},
		"total": {
			Type:        genai.TypeInteger,
			Description: "Total del pedido en pesos, tal como lo devolvió place_order",
		},
		"message": {
			Type:        genai.TypeString,
			Description: "Mensaje para el cliente: la confirmación, el motivo del rechazo o la respuesta a su consulta",
		},
	},
	Required: []string{"status", "order_id", "message"},
}

func main() {
	ctx := context.Background()

	// ----------------------------------------------
	// Creación del modelo
	// ----------------------------------------------
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		fmt.Println("Debes configurar GOOGLE_API_KEY")
		os.Exit(1)
	}

	modelName := os.Getenv("BAKERY_GEMINI_MODEL")
	if modelName == "" {
		modelName = "gemini-3.5-flash-lite"
	}

	clientConfig := genai.ClientConfig{
		APIKey: apiKey,
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	placeOrder, err := functiontool.New(functiontool.Config{
		Name:        "place_order",
		Description: "Toma el pedido de un pan: valida el pan y la cantidad, calcula el total y devuelve el número del pedido.",
	}, (&orders{}).PlaceOrder)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Agente de la panadería
	// El Name y la Description se publican en la agent card: es lo que
	// leen los otros agentes para saber qué hace este agente.
	// ----------------------------------------------
	bakeryAgentConfig := llmagent.Config{
		Name:        "bakery_agent",
		Model:       model,
		Description: "Panadería La Espiga: vende pan a otros negocios. Informa su catálogo y precios, y toma pedidos de un pan y una cantidad.",
		Instruction: `
			Eres el agente de la panadería La Espiga. Atiendes a otros negocios que
			te compran pan. Habla en español, de forma breve.

			Catálogo de hoy (precio por unidad para negocios):
			` + catalogText() + `

			- Si te piden pan, llama a "place_order" con el pan y la cantidad. Si piden
			  varios panes, llámala una vez por cada uno. Nunca calcules el total ni
			  inventes un número de pedido: usa lo que devuelve la herramienta.
			- Responde con el resultado de "place_order" tal cual: status, order_id,
			  product, quantity, total y message.
			- Si solo preguntan por el catálogo o los precios, responde con status
			  "info", order_id 0 y la respuesta en message.
		`,
		Tools:        []tool.Tool{placeOrder},
		OutputSchema: &orderSchema,
	}
	bakeryAgent, err := llmagent.New(bakeryAgentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Con el sublauncher "a2a", el launcher publica la agent card en
	// /.well-known/agent-card.json y atiende las peticiones A2A.
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader: agent.NewSingleLoader(bakeryAgent),
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
