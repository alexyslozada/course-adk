package main

import (
	"context"
	"fmt"
	"net/url"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tipos de salida: los campos JSON coinciden con los de la API de la app.

type Product struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price" jsonschema:"precio en pesos"`
	Active   bool   `json:"active"`
}

type InventoryItem struct {
	ProductID   int    `json:"product_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Quantity    int    `json:"quantity"`
	MinQuantity int    `json:"min_quantity"`
}

type Customer struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SaleItem struct {
	ProductID int    `json:"product_id"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unit_price"`
}

type Sale struct {
	ID              int        `json:"id"`
	Customer        string     `json:"customer"`
	Total           int        `json:"total" jsonschema:"total final en pesos, con el descuento incluido"`
	DiscountPercent int        `json:"discount_percent" jsonschema:"descuento ya aplicado a la venta, en porcentaje"`
	CreatedAt       string     `json:"created_at"`
	Items           []SaleItem `json:"items"`
}

type Review struct {
	ID        int    `json:"id"`
	Customer  string `json:"customer"`
	Product   string `json:"product"`
	Rating    int    `json:"rating"`
	Comment   string `json:"comment"`
	CreatedAt string `json:"created_at"`
}

// registerTools declara las tools del MCP. Cada una es una llamada a la API de la app.
func registerTools(s *mcp.Server, app *appclient.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_menu",
		Description: "Lista los productos del menú con su id, categoría y precio en pesos. Úsala para saber qué se vende y cuánto cuesta.",
	}, listMenu(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_stock",
		Description: "Consulta el inventario: cantidad disponible y mínimo de cada producto. Con low_only=true devuelve solo los que están en el mínimo o por debajo.",
	}, getStock(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_customers",
		Description: "Lista los clientes registrados con su id. Úsala para encontrar el id de un cliente por su nombre.",
	}, listCustomers(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_sales",
		Description: "Lista las últimas ventas, la más reciente primero, con sus productos y total en pesos. Con date devuelve solo las de ese día; el total ya incluye el descuento (discount_percent).",
	}, listSales(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_reviews",
		Description: "Lista las reseñas de los clientes, la más reciente primero, con calificación de 1 a 5. Con date devuelve solo las de ese día.",
	}, listReviews(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_sale",
		Description: "Registra una venta y descuenta el inventario. Necesita los ids de producto (de list_menu) y las cantidades. Falla si no hay stock suficiente. El cliente es opcional.",
	}, createSale(app))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "create_review",
		Description: "Registra una reseña de un cliente con calificación de 1 a 5. El producto y el cliente son opcionales.",
	}, createReview(app))
}

// --- list_menu ---

type menuOut struct {
	Products []Product `json:"products"`
}

func listMenu(app *appclient.Client) mcp.ToolHandlerFor[struct{}, menuOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, menuOut, error) {
		var all []Product
		if err := app.Do(ctx, "GET", "/api/products", nil, &all); err != nil {
			return nil, menuOut{}, err
		}
		out := menuOut{Products: []Product{}}
		for _, p := range all {
			if p.Active {
				out.Products = append(out.Products, p)
			}
		}
		return nil, out, nil
	}
}

// --- get_stock ---

type stockIn struct {
	LowOnly bool `json:"low_only,omitempty" jsonschema:"si es true, solo los productos con stock bajo"`
}

type stockOut struct {
	Items []InventoryItem `json:"items"`
}

func getStock(app *appclient.Client) mcp.ToolHandlerFor[stockIn, stockOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in stockIn) (*mcp.CallToolResult, stockOut, error) {
		var all []InventoryItem
		if err := app.Do(ctx, "GET", "/api/inventory", nil, &all); err != nil {
			return nil, stockOut{}, err
		}
		out := stockOut{Items: []InventoryItem{}}
		for _, i := range all {
			if !in.LowOnly || i.Quantity <= i.MinQuantity {
				out.Items = append(out.Items, i)
			}
		}
		return nil, out, nil
	}
}

// --- list_customers ---

type customersOut struct {
	Customers []Customer `json:"customers"`
}

func listCustomers(app *appclient.Client) mcp.ToolHandlerFor[struct{}, customersOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, customersOut, error) {
		out := customersOut{Customers: []Customer{}}
		if err := app.Do(ctx, "GET", "/api/customers", nil, &out.Customers); err != nil {
			return nil, customersOut{}, err
		}
		return nil, out, nil
	}
}

// --- list_sales ---

type salesIn struct {
	Limit int    `json:"limit,omitempty" jsonschema:"cuántas ventas devolver (por defecto 10, máximo 100)"`
	Date  string `json:"date,omitempty" jsonschema:"solo las ventas de ese día, con formato AAAA-MM-DD (opcional)"`
}

type salesOut struct {
	Sales []Sale `json:"sales"`
}

func listSales(app *appclient.Client) mcp.ToolHandlerFor[salesIn, salesOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in salesIn) (*mcp.CallToolResult, salesOut, error) {
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		limit = min(limit, 100)
		out := salesOut{Sales: []Sale{}}
		query := url.Values{"limit": {fmt.Sprint(limit)}}
		if in.Date != "" {
			query.Set("date", in.Date)
		}
		path := "/api/sales?" + query.Encode()
		if err := app.Do(ctx, "GET", path, nil, &out.Sales); err != nil {
			return nil, salesOut{}, err
		}
		return nil, out, nil
	}
}

// --- list_reviews ---

type reviewsIn struct {
	Date string `json:"date,omitempty" jsonschema:"solo las reseñas de ese día, con formato AAAA-MM-DD (opcional)"`
}

type reviewsOut struct {
	Reviews []Review `json:"reviews"`
}

func listReviews(app *appclient.Client) mcp.ToolHandlerFor[reviewsIn, reviewsOut] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in reviewsIn) (*mcp.CallToolResult, reviewsOut, error) {
		out := reviewsOut{Reviews: []Review{}}
		path := "/api/reviews"
		if in.Date != "" {
			path += "?" + url.Values{"date": {in.Date}}.Encode()
		}
		if err := app.Do(ctx, "GET", path, nil, &out.Reviews); err != nil {
			return nil, reviewsOut{}, err
		}
		return nil, out, nil
	}
}

// --- create_sale ---

type saleLine struct {
	ProductID int `json:"product_id" jsonschema:"id del producto, de list_menu"`
	Quantity  int `json:"quantity" jsonschema:"unidades, mayor a cero"`
}

type saleIn struct {
	CustomerID *int       `json:"customer_id,omitempty" jsonschema:"id del cliente, de list_customers (opcional)"`
	Items      []saleLine `json:"items" jsonschema:"productos de la venta"`
}

func createSale(app *appclient.Client) mcp.ToolHandlerFor[saleIn, Sale] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in saleIn) (*mcp.CallToolResult, Sale, error) {
		var sale Sale
		if err := app.Do(ctx, "POST", "/api/sales", in, &sale); err != nil {
			return nil, Sale{}, err
		}
		return nil, sale, nil
	}
}

// --- create_review ---

type reviewIn struct {
	Rating     int    `json:"rating" jsonschema:"calificación de 1 a 5"`
	Comment    string `json:"comment,omitempty" jsonschema:"comentario del cliente"`
	CustomerID *int   `json:"customer_id,omitempty" jsonschema:"id del cliente (opcional)"`
	ProductID  *int   `json:"product_id,omitempty" jsonschema:"id del producto reseñado (opcional)"`
}

func createReview(app *appclient.Client) mcp.ToolHandlerFor[reviewIn, Review] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in reviewIn) (*mcp.CallToolResult, Review, error) {
		var review Review
		if err := app.Do(ctx, "POST", "/api/reviews", in, &review); err != nil {
			return nil, Review{}, err
		}
		return nil, review, nil
	}
}
