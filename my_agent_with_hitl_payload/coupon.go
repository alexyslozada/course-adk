package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
)

type ApplyCouponRequest struct {
	Product string `json:"product" jsonschema:"Nombre del producto, por ejemplo: camiseta, zapatillas, gorra"`
	Coupon  string `json:"coupon" jsonschema:"Código del cupón, por ejemplo: BIENVENIDO-10"`
}

type ApplyCouponResponse struct {
	Status   string  `json:"status"`
	Product  string  `json:"product"`
	Coupon   string  `json:"coupon"`
	Discount int     `json:"discount"`
	Price    float64 `json:"price"`
	Final    float64 `json:"final"`
}

// ApprovalPayload es lo que devuelve la persona que aprueba: puede modificar
// el porcentaje de descuento antes de aprobar.
type ApprovalPayload struct {
	Discount int `json:"discount"`
}

var products = map[string]float64{
	"camiseta":   20,
	"zapatillas": 80,
	"gorra":      15,
}

// Porcentaje de descuento de cada cupón
var coupons = map[string]int{
	"BIENVENIDO-10": 10,
	"ESPECIAL-50":   50,
}

// Los cupones con un descuento mayor a este umbral requieren aprobación humana
const confirmationThreshold = 20

func ApplyCoupon(ctx agent.Context, req ApplyCouponRequest) (ApplyCouponResponse, error) {
	price, isFound := products[strings.ToLower(req.Product)]
	if !isFound {
		return ApplyCouponResponse{}, fmt.Errorf("no existe el producto: %s", req.Product)
	}

	discount, isFound := coupons[strings.ToUpper(req.Coupon)]
	if !isFound {
		return ApplyCouponResponse{}, fmt.Errorf("el cupón %s no es válido", req.Coupon)
	}

	response := ApplyCouponResponse{
		Status:   "Cupón aplicado",
		Product:  req.Product,
		Coupon:   req.Coupon,
		Discount: discount,
		Price:    price,
	}

	if discount > confirmationThreshold {
		confirmation := ctx.ToolConfirmation()

		// Primera llamada: todavía no hay respuesta humana, así que la pedimos.
		// El ADK detiene al agente hasta que la persona apruebe o rechace.
		if confirmation == nil {
			err := ctx.RequestConfirmation(
				fmt.Sprintf("El cupón %s da %d%% de descuento. Aprueba, rechaza o modifica el descuento.", req.Coupon, discount),
				ApprovalPayload{Discount: discount},
			)
			if err != nil {
				return ApplyCouponResponse{}, err
			}
			response.Status = "Se requiere la aprobación de un supervisor"
			return response, nil
		}

		// Segunda llamada: la persona ya respondió.
		if !confirmation.Confirmed {
			response.Status = "El supervisor rechazó el cupón"
			response.Discount = 0
			response.Final = price
			return response, nil
		}

		// El payload llega como un map genérico; lo convertimos a nuestro struct.
		var payload ApprovalPayload
		jsonBytes, err := json.Marshal(confirmation.Payload)
		if err != nil {
			return ApplyCouponResponse{}, err
		}
		if err := json.Unmarshal(jsonBytes, &payload); err != nil {
			return ApplyCouponResponse{}, err
		}

		// El supervisor puede bajar el descuento, pero nunca subirlo.
		if payload.Discount > 0 && payload.Discount < discount {
			response.Status = "Cupón aplicado con el descuento modificado por el supervisor"
			response.Discount = payload.Discount
		}
	}

	response.Final = price * float64(100-response.Discount) / 100
	return response, nil
}
