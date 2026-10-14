package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/remoteagent/v2"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// La cafetería no hornea: le compra el pan a la panadería La Espiga, otro negocio con su propio
// agente, por A2A. El LLM del administrador no le habla directo a la panadería: lo hace order_bread,
// una tool Go que primero pide la aprobación de una persona (pedir pan cuesta dinero) y luego envía
// el pedido. Así nadie pide pan sin aprobación ni con otra cantidad.

// breadCategory es la categoría de los productos que se le piden a la panadería.
const breadCategory = "Panadería"

// newBakeryAgent crea el agente remoto: solo se conoce su URL, de donde el ADK descarga la agent card.
func newBakeryAgent(url string) (agent.Agent, error) {
	return remoteagent.NewA2A(remoteagent.A2AConfig{
		Name:              "bakery_agent",
		Description:       "Panadería La Espiga: vende pan a otros negocios.",
		AgentCardProvider: sameOriginCardProvider(url),
	})
}

// sameOriginCardProvider descarga la agent card y exige que todas las direcciones que declara sean del
// mismo origen (esquema, host y puerto) que la URL configurada: así una card alterada no puede desviar
// los pedidos a otro servidor.
//
// remoteagent.NewAgentCardProvider hace lo mismo y además exige https (o http solo en localhost). Con
// docker compose la panadería es http://bakery:8001, dentro de la red privada de los contenedores, y
// esa regla la rechazaría. Con una panadería real en internet la URL sería https.
func sameOriginCardProvider(source string) remoteagent.AgentCardProvider {
	return func(ctx context.Context) (*a2a.AgentCard, error) {
		sourceURL, err := url.Parse(source)
		if err != nil {
			return nil, err
		}
		card, err := agentcard.DefaultResolver.Resolve(ctx, source)
		if err != nil {
			return nil, fmt.Errorf("no se pudo descargar la agent card: %w", err)
		}
		for _, iface := range card.SupportedInterfaces {
			if iface == nil {
				continue
			}
			ifaceURL, err := url.Parse(iface.URL)
			if err != nil || ifaceURL.Scheme != sourceURL.Scheme || !strings.EqualFold(ifaceURL.Host, sourceURL.Host) {
				return nil, fmt.Errorf("la agent card apunta a %q, fuera de %s", iface.URL, source)
			}
		}
		return card, nil
	}
}

type restock struct {
	app    *appclient.Client
	bakery agent.Agent
}

type product struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// bakeryOrder es la respuesta de la panadería (su OutputSchema).
type bakeryOrder struct {
	Status   string `json:"status"`
	OrderID  int    `json:"order_id"`
	Product  string `json:"product"`
	Quantity int    `json:"quantity"`
	Total    int    `json:"total"`
	Message  string `json:"message"`
}

type OrderBreadRequest struct {
	ProductID int `json:"product_id" jsonschema:"id del producto, de get_inventory; debe ser de panadería"`
	Quantity  int `json:"quantity" jsonschema:"unidades que se le piden a la panadería (se suman al inventario actual)"`
}

// RestockPayload es lo que devuelve la persona que aprueba: puede cambiar la cantidad.
type RestockPayload struct {
	Quantity int `json:"quantity"`
}

type OrderBreadResponse struct {
	Status        string `json:"status"`
	Product       string `json:"product,omitempty"`
	Quantity      int    `json:"quantity,omitempty"`
	BakeryOrderID int    `json:"bakery_order_id,omitempty"`
	Total         int    `json:"total,omitempty" jsonschema:"lo que cobra la panadería, en pesos"`
	Before        int    `json:"before"`
	After         int    `json:"after"`
}

func (r restock) OrderBread(ctx agent.Context, req OrderBreadRequest) (OrderBreadResponse, error) {
	var products []product
	if err := r.app.Do(ctx, "GET", "/api/products", nil, &products); err != nil {
		return OrderBreadResponse{}, err
	}
	var breads []string
	var target *product
	for i := range products {
		if products[i].Category == breadCategory {
			breads = append(breads, products[i].Name)
		}
		if products[i].ID == req.ProductID {
			target = &products[i]
		}
	}
	if target == nil {
		return OrderBreadResponse{}, fmt.Errorf("no existe el producto %d", req.ProductID)
	}
	if target.Category != breadCategory {
		return OrderBreadResponse{}, fmt.Errorf("%s no se le pide a la panadería; solo los productos de panadería: %s", target.Name, strings.Join(breads, ", "))
	}

	current, err := r.inventoryOf(ctx, req.ProductID)
	if err != nil {
		return OrderBreadResponse{}, err
	}
	response := OrderBreadResponse{Product: target.Name, Before: current.Quantity, After: current.Quantity}

	confirmation := ctx.ToolConfirmation()
	// Primera llamada: se pide la aprobación y el ADK pausa al agente.
	if confirmation == nil {
		if req.Quantity < 1 {
			return OrderBreadResponse{}, fmt.Errorf("la cantidad debe ser mayor que 0")
		}
		hint := fmt.Sprintf("¿Pedirle %d unidades de %s a la panadería La Espiga? Hoy hay %d (mínimo %d). Puedes cambiar la cantidad.",
			req.Quantity, target.Name, current.Quantity, current.MinQuantity)
		if err := ctx.RequestConfirmation(hint, RestockPayload{Quantity: req.Quantity}); err != nil {
			return OrderBreadResponse{}, err
		}
		response.Status = "Pendiente de la aprobación del administrador"
		return response, nil
	}
	// Segunda llamada: la persona ya respondió.
	if !confirmation.Confirmed {
		response.Status = "El administrador rechazó el pedido; no se le pidió nada a la panadería"
		return response, nil
	}

	// El payload llega como un map genérico; se convierte a nuestro struct.
	payload := RestockPayload{Quantity: req.Quantity}
	if confirmation.Payload != nil {
		raw, err := json.Marshal(confirmation.Payload)
		if err != nil {
			return OrderBreadResponse{}, err
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return OrderBreadResponse{}, err
		}
	}
	if payload.Quantity < 1 {
		return OrderBreadResponse{}, fmt.Errorf("la cantidad debe ser mayor que 0")
	}

	// El pedido viaja por A2A. La panadería responde con su OutputSchema: un JSON, no texto libre.
	reply, err := runAgent(ctx, r.bakery, fmt.Sprintf("Pedido de la cafetería: %d unidades de %s.", payload.Quantity, target.Name))
	if err != nil {
		return OrderBreadResponse{}, fmt.Errorf("no se pudo hablar con la panadería: %w", err)
	}
	order, err := parseBakeryOrder(reply)
	if err != nil {
		return OrderBreadResponse{}, err
	}
	if order.Status != "confirmado" {
		response.Status = "La panadería no tomó el pedido: " + order.Message
		return response, nil
	}
	// Lo que se suma al inventario es lo que la panadería confirmó, y debe ser lo que se pidió.
	if !strings.EqualFold(order.Product, target.Name) || order.Quantity != payload.Quantity {
		return OrderBreadResponse{}, fmt.Errorf("la panadería confirmó otro pedido (%d de %s, número %d): revísalo con ella; el inventario no cambió",
			order.Quantity, order.Product, order.OrderID)
	}

	// La panadería entrega en el día: el pan se suma al inventario de una vez.
	// Se vuelve a leer el inventario: pudo cambiar mientras se esperaba la aprobación.
	current, err = r.inventoryOf(ctx, req.ProductID)
	if err != nil {
		return OrderBreadResponse{}, err
	}
	body := map[string]int{"quantity": current.Quantity + order.Quantity, "min_quantity": current.MinQuantity}
	var updated InventoryItem
	if err := r.app.Do(ctx, "PUT", fmt.Sprintf("/api/inventory/%d", req.ProductID), body, &updated); err != nil {
		return OrderBreadResponse{}, fmt.Errorf("la panadería confirmó el pedido %d, pero no se pudo actualizar el inventario: %w", order.OrderID, err)
	}

	response.Status = "Pedido confirmado por la panadería e inventario actualizado"
	response.Quantity = order.Quantity
	response.BakeryOrderID = order.OrderID
	response.Total = order.Total
	response.Before = current.Quantity
	response.After = updated.Quantity
	return response, nil
}

func (r restock) inventoryOf(ctx agent.Context, productID int) (InventoryItem, error) {
	var items []InventoryItem
	if err := r.app.Do(ctx, "GET", "/api/inventory", nil, &items); err != nil {
		return InventoryItem{}, err
	}
	for _, it := range items {
		if it.ProductID == productID {
			return it, nil
		}
	}
	return InventoryItem{}, fmt.Errorf("el producto %d no está en el inventario", productID)
}

// parseBakeryOrder lee el JSON de la panadería, aunque venga entre ```json ... ```.
func parseBakeryOrder(reply string) (bakeryOrder, error) {
	var order bakeryOrder
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end < start {
		return order, fmt.Errorf("la panadería respondió algo que no es un pedido: %q", reply)
	}
	if err := json.Unmarshal([]byte(reply[start:end+1]), &order); err != nil {
		return order, fmt.Errorf("la panadería respondió algo que no es un pedido: %w", err)
	}
	return order, nil
}

func (r restock) tool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "order_bread",
		Description: "Le pide pan a la panadería La Espiga por A2A y suma lo que entrega al inventario. Pide la aprobación del administrador, que puede cambiar la cantidad.",
	}, r.OrderBread)
}
