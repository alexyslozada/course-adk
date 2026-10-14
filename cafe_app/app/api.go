package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("escribiendo JSON: %v", err)
	}
}

// apiError traduce un error del store a la respuesta HTTP correspondiente.
func apiError(w http.ResponseWriter, err error) {
	input, isInput := errors.AsType[InputError](err)
	switch {
	case isInput:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": input.Error()})
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	default:
		log.Printf("error interno: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "error interno"})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return false
	}
	return true
}

func (a *App) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", a.apiLogin)
	mux.HandleFunc("GET /api/products", a.requireAPI(a.apiListProducts))
	mux.HandleFunc("POST /api/products", a.requireAPI(a.apiCreateProduct))
	mux.HandleFunc("GET /api/inventory", a.requireAPI(a.apiListInventory))
	mux.HandleFunc("PUT /api/inventory/{product_id}", a.requireAPI(a.apiSetInventory))
	mux.HandleFunc("GET /api/customers", a.requireAPI(a.apiListCustomers))
	mux.HandleFunc("POST /api/customers", a.requireAPI(a.apiCreateCustomer))
	mux.HandleFunc("GET /api/sales", a.requireAPI(a.apiListSales))
	mux.HandleFunc("POST /api/sales", a.requireAPI(a.apiCreateSale))
	mux.HandleFunc("GET /api/reviews", a.requireAPI(a.apiListReviews))
	mux.HandleFunc("POST /api/reviews", a.requireAPI(a.apiCreateReview))
}

// apiLogin entrega el JWT para usarlo como "Authorization: Bearer <token>".
func (a *App) apiLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	user, ok := a.authenticate(r, in.Username, in.Password)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "usuario o contraseña incorrectos"})
		return
	}
	token, err := a.issueToken(user)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "expires_in": int(tokenTTL.Seconds()), "user": user})
}

func (a *App) apiListProducts(w http.ResponseWriter, r *http.Request) {
	products, err := a.store.ListProducts(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (a *App) apiCreateProduct(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Price    int    `json:"price"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := a.store.CreateProduct(r.Context(), in.Name, in.Category, in.Price)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *App) apiListInventory(w http.ResponseWriter, r *http.Request) {
	items, err := a.store.ListInventory(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *App) apiSetInventory(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.Atoi(r.PathValue("product_id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "product_id inválido"})
		return
	}
	var in struct {
		Quantity    int `json:"quantity"`
		MinQuantity int `json:"min_quantity"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	item, err := a.store.SetInventory(r.Context(), productID, in.Quantity, in.MinQuantity)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (a *App) apiListCustomers(w http.ResponseWriter, r *http.Request) {
	customers, err := a.store.ListCustomers(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, customers)
}

func (a *App) apiCreateCustomer(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := a.store.CreateCustomer(r.Context(), in.Name, in.Email)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (a *App) apiListSales(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	sales, err := a.store.ListSales(r.Context(), limit)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sales)
}

func (a *App) apiCreateSale(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CustomerID *int       `json:"customer_id"`
		Items      []SaleLine `json:"items"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	sale, err := a.store.CreateSale(r.Context(), userFrom(r.Context()).ID, in.CustomerID, in.Items)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sale)
}

func (a *App) apiListReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := a.store.ListReviews(r.Context())
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}

func (a *App) apiCreateReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CustomerID *int   `json:"customer_id"`
		ProductID  *int   `json:"product_id"`
		Rating     int    `json:"rating"`
		Comment    string `json:"comment"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	review, err := a.store.CreateReview(r.Context(), in.CustomerID, in.ProductID, in.Rating, in.Comment)
	if err != nil {
		apiError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, review)
}
