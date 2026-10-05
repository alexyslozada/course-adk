package main

import (
	"fmt"
	"strings"

	"google.golang.org/adk/v2/agent"
)

type ApplyCouponRequest struct {
	Product string `json:"product" jsonschema:"Nombre del producto, por ejemplo: camiseta, zapatillas, gorra"`
	Coupon  string `json:"coupon" jsonschema:"Código del cupón, por ejemplo: BIENVENIDO-10"`
}

type ApplyCouponResponse struct {
	Product  string  `json:"product"`
	Coupon   string  `json:"coupon"`
	Discount int     `json:"discount"`
	Price    float64 `json:"price"`
	Final    float64 `json:"final"`
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

// Los cupones con un descuento mayor a este umbral requieren confirmación humana
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

	return ApplyCouponResponse{
		Product:  req.Product,
		Coupon:   req.Coupon,
		Discount: discount,
		Price:    price,
		Final:    price * float64(100-discount) / 100,
	}, nil
}

// RequiresConfirmation decide, antes de ejecutar la tool, si se le debe pedir
// confirmación al usuario: solo cuando el cupón supera el umbral de descuento.
func RequiresConfirmation(req ApplyCouponRequest) bool {
	return coupons[strings.ToUpper(req.Coupon)] > confirmationThreshold
}
