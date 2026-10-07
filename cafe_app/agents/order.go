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

// customerNameKey es la clave del estado donde queda el nombre del cliente. Cada tarea del barista ve
// solo su propia conversación, así que el nombre dado en un pedido se perdía en el siguiente. El estado
// de la sesión sí lo comparten todas las tareas (y se guarda en SQLite con la sesión).
const customerNameKey = "customer_name"

// register_sale es la única forma en que el barista crea una venta. A diferencia de la tool
// create_sale del MCP, es una función Go: puede pausar al agente con ctx.RequestConfirmation,
// así que la confirmación del cliente la exige el código y no depende de que el modelo se acuerde.
// Además calcula el total y revisa el stock en código: el modelo no hace cuentas.
type order struct {
	app *appclient.Client
}

type customer struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type OrderLine struct {
	ProductID int `json:"product_id" jsonschema:"id del producto, de list_menu"`
	Quantity  int `json:"quantity" jsonschema:"unidades pedidas, exactamente las que pidió el cliente"`
}

type RegisterSaleRequest struct {
	CustomerName string      `json:"customer_name" jsonschema:"nombre que te dio el cliente, tal cual; si no quiere darlo, escribe: anónimo"`
	Items        []OrderLine `json:"items" jsonschema:"productos y cantidades del pedido"`
}

type RegisterSaleResponse struct {
	Status  string   `json:"status"`
	SaleID  int      `json:"sale_id"`
	Total   int      `json:"total"`
	Summary string   `json:"summary,omitempty"`
	Problem []string `json:"problems,omitempty"`
}

func (o order) RegisterSale(ctx agent.Context, req RegisterSaleRequest) (RegisterSaleResponse, error) {
	// El nombre lo exige el código: el modelo no puede saltarse la pregunta ni inventar uno.
	name := strings.TrimSpace(req.CustomerName)
	if name == "" || !customerSaid(ctx, name) {
		return RegisterSaleResponse{Status: "El cliente todavía no ha dicho su nombre (o que prefiere no darlo): pregúntaselo antes de registrar"}, nil
	}
	// Desde aquí el nombre es válido: se recuerda para los siguientes pedidos de la conversación.
	if !isAnonymous(name) {
		if err := ctx.State().Set(customerNameKey, name); err != nil {
			return RegisterSaleResponse{}, err
		}
	}

	var products []menuProduct
	if err := o.app.Do(ctx, "GET", "/api/products", nil, &products); err != nil {
		return RegisterSaleResponse{}, err
	}
	var inventory []InventoryItem
	if err := o.app.Do(ctx, "GET", "/api/inventory", nil, &inventory); err != nil {
		return RegisterSaleResponse{}, err
	}
	byID := map[int]menuProduct{}
	for _, p := range products {
		byID[p.ID] = p
	}
	stock := map[int]int{}
	for _, i := range inventory {
		stock[i.ProductID] = i.Quantity
	}

	// Se juntan los renglones repetidos y se valida todo en código.
	quantities := map[int]int{}
	var ids []int
	var problems []string
	for _, line := range req.Items {
		p, found := byID[line.ProductID]
		switch {
		case !found || !p.Active:
			problems = append(problems, fmt.Sprintf("el producto %d no está en el menú", line.ProductID))
			continue
		case line.Quantity <= 0:
			problems = append(problems, fmt.Sprintf("la cantidad de %s debe ser mayor a cero", p.Name))
			continue
		}
		if _, seen := quantities[p.ID]; !seen {
			ids = append(ids, p.ID)
		}
		quantities[p.ID] += line.Quantity
	}
	if len(ids) == 0 && len(problems) == 0 {
		problems = append(problems, "el pedido no tiene productos")
	}
	total := 0
	var lines []string
	for _, id := range ids {
		p, qty := byID[id], quantities[id]
		if qty > stock[id] {
			problems = append(problems, fmt.Sprintf("%s: pides %d y solo hay %d", p.Name, qty, stock[id]))
		}
		total += p.Price * qty
		lines = append(lines, fmt.Sprintf("%d x %s", qty, p.Name))
	}
	// Si el pedido no se puede registrar, no se le pide nada al cliente: se explica por qué.
	if len(problems) > 0 {
		return RegisterSaleResponse{Status: "No se puede registrar el pedido", Problem: problems}, nil
	}
	summary := strings.Join(lines, ", ")

	// El cliente se busca por nombre en la app; si no está registrado, la venta va sin cliente.
	var customers []customer
	if err := o.app.Do(ctx, "GET", "/api/customers", nil, &customers); err != nil {
		return RegisterSaleResponse{}, err
	}
	customerID, who := 0, name+" (no registrado)"
	if isAnonymous(name) {
		who = "anónimo"
	} else if i := slices.IndexFunc(customers, func(c customer) bool { return strings.EqualFold(c.Name, name) }); i >= 0 {
		customerID, who = customers[i].ID, customers[i].Name
	}

	confirmation := ctx.ToolConfirmation()
	// Primera llamada: se pide la confirmación y el ADK pausa al agente.
	if confirmation == nil {
		hint := fmt.Sprintf("¿Registrar este pedido? %s. Cliente: %s. Total: %s.", summary, who, pesos(total))
		if err := ctx.RequestConfirmation(hint, nil); err != nil {
			return RegisterSaleResponse{}, err
		}
		return RegisterSaleResponse{Status: "Pendiente de la confirmación del cliente", Total: total, Summary: summary}, nil
	}
	// Segunda llamada: el cliente aprobó. Si rechaza, el ADK ni siquiera vuelve a ejecutar la tool:
	// el modelo recibe un error "call is rejected" (ver las instrucciones del barista).
	if !confirmation.Confirmed {
		return RegisterSaleResponse{Status: "El cliente canceló el pedido; no se registró nada", Summary: summary}, nil
	}

	body := map[string]any{"items": req.Items}
	if customerID > 0 {
		body["customer_id"] = customerID
	}
	var created sale
	if err := o.app.Do(ctx, "POST", "/api/sales", body, &created); err != nil {
		return RegisterSaleResponse{}, err
	}
	return RegisterSaleResponse{Status: "Venta registrada", SaleID: created.ID, Total: created.Total, Summary: summary}, nil
}

// fold pasa a minúsculas y quita las tildes para comparar textos.
var fold = func() func(string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	return func(s string) string { return r.Replace(strings.ToLower(s)) }
}()

func isAnonymous(name string) bool { return strings.Contains(fold(name), "anonim") }

// customerSaid comprueba que el cliente dijo ese nombre: lo recuerda el estado de un pedido anterior
// o aparece en la conversación guardada, o que pidió no darlo si el nombre es "anónimo". Sin conversación guardada no puede
// comprobarlo y lo acepta.
func customerSaid(ctx agent.Context, name string) bool {
	if known, err := ctx.State().Get(customerNameKey); err == nil && fold(fmt.Sprint(known)) == fold(name) {
		return true
	}
	history, err := ctx.State().Get(conversationKey)
	if err != nil {
		return true
	}
	var said strings.Builder
	for line := range strings.SplitSeq(fmt.Sprint(history), "\n") {
		if rest, ok := strings.CutPrefix(line, "Cliente: "); ok {
			fmt.Fprintln(&said, fold(rest))
		}
	}
	if isAnonymous(name) {
		for _, phrase := range []string{"anonim", "sin nombre", "no quiero dar", "prefiero no", "no te lo doy", "no importa"} {
			if strings.Contains(said.String(), phrase) {
				return true
			}
		}
		return false
	}
	return strings.Contains(said.String(), fold(name))
}

// tool convierte la función en una tool del ADK.
func (o order) tool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "register_sale",
		Description: "Registra la venta de un pedido y descuenta el inventario. Calcula el total y revisa el stock; luego pide la confirmación del cliente con una tarjeta antes de registrar.",
	}, o.RegisterSale)
}
