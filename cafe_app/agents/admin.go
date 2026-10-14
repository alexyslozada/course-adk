package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// Las tools del administrador son funciones Go que llaman directo a la API de la app
// (a diferencia del barista, que usa el MCP). Las que cambian datos piden confirmación humana.

type InventoryItem struct {
	ProductID   int    `json:"product_id"`
	Name        string `json:"name"`
	Quantity    int    `json:"quantity"`
	MinQuantity int    `json:"min_quantity"`
}

type sale struct {
	ID              int    `json:"id"`
	Customer        string `json:"customer"`
	Total           int    `json:"total"`
	DiscountPercent int    `json:"discount_percent"`
}

type admin struct {
	app *appclient.Client
}

// ----------------------------------------------
// get_inventory: solo lee, no pide confirmación
// ----------------------------------------------

type GetInventoryRequest struct{}

type GetInventoryResponse struct {
	Items []InventoryItem `json:"items"`
}

func (a admin) GetInventory(ctx agent.Context, _ GetInventoryRequest) (GetInventoryResponse, error) {
	var items []InventoryItem
	if err := a.app.Do(ctx, "GET", "/api/inventory", nil, &items); err != nil {
		return GetInventoryResponse{}, err
	}
	return GetInventoryResponse{Items: items}, nil
}

// ----------------------------------------------
// adjust_inventory: HITL booleano (aprobar o rechazar)
// ----------------------------------------------

type AdjustInventoryRequest struct {
	ProductID int    `json:"product_id" jsonschema:"id del producto, de get_inventory"`
	Quantity  int    `json:"quantity" jsonschema:"la cantidad total que debe quedar en el inventario (no la diferencia)"`
	Reason    string `json:"reason" jsonschema:"motivo del ajuste, por ejemplo: llegó pedido del proveedor"`
}

type AdjustInventoryResponse struct {
	Status  string `json:"status"`
	Product string `json:"product,omitempty"`
	Before  int    `json:"before"`
	After   int    `json:"after"`
}

func (a admin) AdjustInventory(ctx agent.Context, req AdjustInventoryRequest) (AdjustInventoryResponse, error) {
	var items []InventoryItem
	if err := a.app.Do(ctx, "GET", "/api/inventory", nil, &items); err != nil {
		return AdjustInventoryResponse{}, err
	}
	var current *InventoryItem
	for i := range items {
		if items[i].ProductID == req.ProductID {
			current = &items[i]
		}
	}
	if current == nil {
		return AdjustInventoryResponse{}, fmt.Errorf("no existe el producto %d", req.ProductID)
	}
	response := AdjustInventoryResponse{Product: current.Name, Before: current.Quantity, After: current.Quantity}

	confirmation := ctx.ToolConfirmation()
	// Primera llamada: nadie ha respondido, así que se pide la confirmación y el ADK pausa al agente.
	if confirmation == nil {
		hint := fmt.Sprintf("¿Cambiar el inventario de %s de %d a %d unidades? Motivo: %s.",
			current.Name, current.Quantity, req.Quantity, req.Reason)
		if err := ctx.RequestConfirmation(hint, nil); err != nil {
			return AdjustInventoryResponse{}, err
		}
		response.Status = "Pendiente de la confirmación del administrador"
		return response, nil
	}
	// Segunda llamada: la persona ya respondió.
	if !confirmation.Confirmed {
		response.Status = "El administrador rechazó el ajuste; el inventario no cambió"
		return response, nil
	}

	body := map[string]int{"quantity": req.Quantity, "min_quantity": current.MinQuantity}
	var updated InventoryItem
	if err := a.app.Do(ctx, "PUT", fmt.Sprintf("/api/inventory/%d", req.ProductID), body, &updated); err != nil {
		return AdjustInventoryResponse{}, err
	}
	response.Status = "Inventario ajustado"
	response.After = updated.Quantity
	return response, nil
}

// ----------------------------------------------
// approve_discount: HITL con modificación (aprobar, rechazar o cambiar el porcentaje)
// ----------------------------------------------

type ApproveDiscountRequest struct {
	SaleID  int    `json:"sale_id" jsonschema:"número de la venta, de la lista de ventas"`
	Percent int    `json:"percent" jsonschema:"porcentaje de descuento que se propone, de 1 a 50"`
	Reason  string `json:"reason" jsonschema:"motivo del descuento, por ejemplo: cliente frecuente"`
}

// ApprovalPayload es lo que devuelve la persona que aprueba: puede cambiar el porcentaje.
type ApprovalPayload struct {
	Percent int `json:"percent"`
}

type ApproveDiscountResponse struct {
	Status    string `json:"status"`
	SaleID    int    `json:"sale_id"`
	Percent   int    `json:"percent"`
	Total     int    `json:"total"`
	NewTotal  int    `json:"new_total"`
	Requested int    `json:"requested_percent"`
}

func (a admin) ApproveDiscount(ctx agent.Context, req ApproveDiscountRequest) (ApproveDiscountResponse, error) {
	var s sale
	if err := a.app.Do(ctx, "GET", fmt.Sprintf("/api/sales/%d", req.SaleID), nil, &s); err != nil {
		return ApproveDiscountResponse{}, err
	}
	if s.DiscountPercent > 0 {
		return ApproveDiscountResponse{}, fmt.Errorf("la venta %d ya tiene un descuento del %d%%", s.ID, s.DiscountPercent)
	}
	response := ApproveDiscountResponse{SaleID: s.ID, Total: s.Total, NewTotal: s.Total, Requested: req.Percent}

	confirmation := ctx.ToolConfirmation()
	if confirmation == nil {
		hint := fmt.Sprintf("Descuento del %d%% a la venta %d (%s, total %s). Motivo: %s. Aprueba, rechaza o cambia el porcentaje.",
			req.Percent, s.ID, customerName(s), pesos(s.Total), req.Reason)
		if err := ctx.RequestConfirmation(hint, ApprovalPayload{Percent: req.Percent}); err != nil {
			return ApproveDiscountResponse{}, err
		}
		response.Status = "Pendiente de la aprobación del administrador"
		return response, nil
	}
	if !confirmation.Confirmed {
		response.Status = "El administrador rechazó el descuento; la venta no cambió"
		return response, nil
	}

	// El payload llega como un map genérico; se convierte a nuestro struct.
	payload := ApprovalPayload{Percent: req.Percent}
	if confirmation.Payload != nil {
		raw, err := json.Marshal(confirmation.Payload)
		if err != nil {
			return ApproveDiscountResponse{}, err
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return ApproveDiscountResponse{}, err
		}
	}

	// La app valida el rango (1 a 50) y que la venta no tenga ya un descuento.
	var updated sale
	if err := a.app.Do(ctx, "POST", fmt.Sprintf("/api/sales/%d/discount", req.SaleID), payload, &updated); err != nil {
		return ApproveDiscountResponse{}, err
	}
	response.Status = "Descuento aplicado"
	if payload.Percent != req.Percent {
		response.Status = "Descuento aplicado con el porcentaje modificado por el administrador"
	}
	response.Percent = updated.DiscountPercent
	response.NewTotal = updated.Total
	return response, nil
}

// pesos da formato a un valor, por ejemplo 18000 -> "$ 18.000".
func pesos(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return "$ " + s
}

func customerName(s sale) string {
	if s.Customer == "" {
		return "sin cliente"
	}
	return s.Customer
}

// tools convierte las funciones en tools del ADK.
func (a admin) tools() ([]tool.Tool, error) {
	getInventory, err := functiontool.New(functiontool.Config{
		Name:        "get_inventory",
		Description: "Devuelve el inventario: id, nombre, cantidad y mínimo de cada producto.",
	}, a.GetInventory)
	if err != nil {
		return nil, err
	}
	adjustInventory, err := functiontool.New(functiontool.Config{
		Name:        "adjust_inventory",
		Description: "Cambia la cantidad de un producto en el inventario. Pide la confirmación del administrador antes de aplicar el cambio.",
	}, a.AdjustInventory)
	if err != nil {
		return nil, err
	}
	approveDiscount, err := functiontool.New(functiontool.Config{
		Name:        "approve_discount",
		Description: "Aplica un descuento a una venta ya registrada. Pide la aprobación del administrador, que puede cambiar el porcentaje.",
	}, a.ApproveDiscount)
	if err != nil {
		return nil, err
	}
	return []tool.Tool{getInventory, adjustInventory, approveDiscount}, nil
}
