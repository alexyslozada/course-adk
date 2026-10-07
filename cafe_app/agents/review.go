package main

import (
	"fmt"
	"slices"
	"strings"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// save_review guarda la reseña de un cliente. Es una tool Go para validar en código (la calificación
// debe ser de 1 a 5 y el producto debe existir). Se guarda directo, sin tarjeta de aprobación: el
// cliente ya dijo qué opina y cuánto le da, y una reseña no mueve dinero ni inventario.
type reviewer struct {
	app *appclient.Client
}

type SaveReviewRequest struct {
	Rating       int    `json:"rating" jsonschema:"calificación de 1 a 5 que dio el cliente; nunca la inventes: si no la dijo, pregúntasela"`
	Comment      string `json:"comment" jsonschema:"lo que escribió el cliente, tal cual"`
	Product      string `json:"product,omitempty" jsonschema:"nombre del producto reseñado, solo si el cliente habló de uno"`
	CustomerName string `json:"customer_name,omitempty" jsonschema:"nombre del cliente, si lo conoces"`
}

type SaveReviewResponse struct {
	Status   string `json:"status"`
	ReviewID int    `json:"review_id"`
}

func (r reviewer) SaveReview(ctx agent.Context, req SaveReviewRequest) (SaveReviewResponse, error) {
	comment := strings.TrimSpace(req.Comment)
	if req.Rating < 1 || req.Rating > 5 {
		return SaveReviewResponse{Status: "La calificación debe ser un número de 1 a 5: pregúntasela al cliente"}, nil
	}
	if comment == "" {
		return SaveReviewResponse{Status: "Falta el comentario del cliente"}, nil
	}

	// El producto, si lo hay, se busca por nombre en el menú.
	productID := 0
	if p := strings.TrimSpace(req.Product); p != "" {
		var products []menuProduct
		if err := r.app.Do(ctx, "GET", "/api/products", nil, &products); err != nil {
			return SaveReviewResponse{}, err
		}
		i := slices.IndexFunc(products, func(m menuProduct) bool { return fold(m.Name) == fold(p) })
		if i < 0 {
			return SaveReviewResponse{Status: fmt.Sprintf("El producto %q no está en el menú: pregúntale cuál es", p)}, nil
		}
		productID = products[i].ID
	}

	// El cliente: el nombre que llegó, o el que ya se conoce de un pedido en la conversación.
	name := strings.TrimSpace(req.CustomerName)
	if known, err := ctx.State().Get(customerNameKey); name == "" && err == nil {
		name = fmt.Sprint(known)
	}
	customerID := 0
	if name != "" && !isAnonymous(name) {
		var customers []customer
		if err := r.app.Do(ctx, "GET", "/api/customers", nil, &customers); err != nil {
			return SaveReviewResponse{}, err
		}
		if i := slices.IndexFunc(customers, func(c customer) bool { return fold(c.Name) == fold(name) }); i >= 0 {
			customerID = customers[i].ID
		}
	}

	body := map[string]any{"rating": req.Rating, "comment": comment}
	if productID > 0 {
		body["product_id"] = productID
	}
	if customerID > 0 {
		body["customer_id"] = customerID
	}
	var created struct {
		ID int `json:"id"`
	}
	if err := r.app.Do(ctx, "POST", "/api/reviews", body, &created); err != nil {
		return SaveReviewResponse{}, err
	}
	return SaveReviewResponse{Status: "Reseña guardada", ReviewID: created.ID}, nil
}

func (r reviewer) tool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "save_review",
		Description: "Guarda la reseña de un cliente. Valida la calificación (1 a 5) y el producto.",
	}, r.SaveReview)
}
