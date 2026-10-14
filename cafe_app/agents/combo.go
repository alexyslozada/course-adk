package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagents/loopagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// El combo se resuelve en tres pasos:
//
//  1. combo_builder (Go) calcula el combo base: el más simple y predecible, con la bebida y el
//     alimento más baratos que tengan stock, uno de cada uno por persona. Es un cálculo, no hace
//     falta IA: si no alcanza el presupuesto, lo sabe al instante y dice cuál es el mínimo.
//  2. upsell_loop (loopagent) busca una mejora atractiva dentro del presupuesto. Eso sí es creativo:
//     un modelo la propone y el código la verifica, en bucle, hasta que una cumple.
//  3. combo_result (Go) arma la respuesta: el combo base y, si la hay, la opción mejorada.

type combos struct {
	app *appclient.Client
}

type menuProduct struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	Active   bool   `json:"active"`
}

// categoryDrinks es la categoría de las bebidas; todo lo demás (panadería, comida, postres) cuenta como alimento.
const categoryDrinks = "Bebidas"

type ComboItem struct {
	Product  string `json:"product" jsonschema:"Nombre del producto tal como aparece en el menú"`
	Quantity int    `json:"quantity" jsonschema:"Cantidad del producto"`
}

type ComboLine struct {
	Product   string `json:"product"`
	Quantity  int    `json:"quantity"`
	UnitPrice int    `json:"unit_price"`
	Subtotal  int    `json:"subtotal"`
}

// comboPlan es un combo ya calculado.
type comboPlan struct {
	Lines []ComboLine `json:"lines"`
	Total int         `json:"total"`
}

// Claves del estado con el prefijo temp: solo sirven mientras se arma este combo. El ADK las
// descarta al terminar el turno, así que nunca se confunden con las de un combo anterior.
const (
	comboParamsKey = "temp:combo_params"
	comboBaseKey   = "temp:combo_base"
	upsellCheckKey = "temp:upsell_check"
)

func setJSON(ctx agent.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return ctx.State().Set(key, string(raw))
}

func getJSON(ctx agent.Context, key string, v any) error {
	raw, err := ctx.State().Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(fmt.Append(nil, raw), v)
}

// catalog trae los productos activos y el stock de la app.
func (c combos) catalog(ctx agent.Context) ([]menuProduct, map[int]int, error) {
	var all []menuProduct
	if err := c.app.Do(ctx, "GET", "/api/products", nil, &all); err != nil {
		return nil, nil, err
	}
	var inventory []InventoryItem
	if err := c.app.Do(ctx, "GET", "/api/inventory", nil, &inventory); err != nil {
		return nil, nil, err
	}
	stock := map[int]int{}
	for _, i := range inventory {
		stock[i.ProductID] = i.Quantity
	}
	products := slices.DeleteFunc(all, func(p menuProduct) bool { return !p.Active })
	slices.SortFunc(products, func(a, b menuProduct) int { return cmp.Or(cmp.Compare(a.Price, b.Price), cmp.Compare(a.Name, b.Name)) })
	return products, stock, nil
}

// isFood dice si el producto cuenta como alimento con las preferencias del cliente (sin
// preferencias, cualquier alimento sirve).
func isFood(p menuProduct, categories []string) bool {
	if p.Category == categoryDrinks {
		return false
	}
	return len(categories) == 0 || slices.ContainsFunc(categories, func(c string) bool { return fold(c) == fold(p.Category) })
}

// cheapest toma las n unidades más baratas de la lista, sin pasar el stock de cada producto.
// Devuelve cuántas unidades faltaron si no alcanzó el stock.
func cheapest(list []menuProduct, stock map[int]int, n int) ([]ComboLine, int, int) {
	var lines []ComboLine
	total := 0
	for _, p := range list {
		if n == 0 {
			break
		}
		q := min(stock[p.ID], n)
		if q <= 0 {
			continue
		}
		lines = append(lines, ComboLine{Product: p.Name, Quantity: q, UnitPrice: p.Price, Subtotal: q * p.Price})
		total += q * p.Price
		n -= q
	}
	return lines, total, n
}

// minUpgrade es lo mínimo que cuesta mejorar el combo base: agregar una unidad de algo con stock
// o cambiar una unidad por otro producto más caro del mismo tipo. Si el margen no alcanza ni para
// eso, no tiene sentido buscar una mejora.
func minUpgrade(products []menuProduct, stock map[int]int, base comboPlan, categories []string) int {
	used := map[string]int{}
	for _, l := range base.Lines {
		used[l.Product] = l.Quantity
	}
	best := math.MaxInt
	for _, q := range products {
		if stock[q.ID]-used[q.Name] <= 0 {
			continue
		}
		if q.Category == categoryDrinks || isFood(q, categories) {
			best = min(best, q.Price) // agregar una unidad
		}
		for _, l := range base.Lines { // cambiar una unidad del combo por q
			p := products[slices.IndexFunc(products, func(m menuProduct) bool { return m.Name == l.Product })]
			sameKind := (p.Category == categoryDrinks) == (q.Category == categoryDrinks)
			if sameKind && (q.Category == categoryDrinks || isFood(q, categories)) && q.Price > p.Price {
				best = min(best, q.Price-p.Price)
			}
		}
	}
	return best
}

func describe(plan comboPlan) string {
	var b strings.Builder
	for _, l := range plan.Lines {
		fmt.Fprintf(&b, "- %d x %s (%s)\n", l.Quantity, l.Product, pesos(l.Subtotal))
	}
	return b.String()
}

// Build es el nodo combo_builder: valida los datos del combo, calcula el combo base y decide si
// vale la pena buscar una mejora.
func (c combos) Build(ctx agent.Context, input any, emit func(*session.Event) error) (string, error) {
	var p comboParams
	if err := decodeNodeInput(input, &p); err != nil {
		return say(ctx, emit, "No entendí tu solicitud de combo. ¿Para cuántas personas es y cuál es tu presupuesto?")
	}
	var missing []string
	if p.People <= 0 {
		missing = append(missing, "para cuántas personas es")
	}
	if p.Budget <= 0 {
		missing = append(missing, "cuál es tu presupuesto")
	}
	if len(missing) > 0 {
		return say(ctx, emit, "Para armar el combo necesito saber "+strings.Join(missing, " y ")+".")
	}

	products, stock, err := c.catalog(ctx)
	if err != nil {
		return "", err
	}
	drinks := slices.DeleteFunc(slices.Clone(products), func(m menuProduct) bool { return m.Category != categoryDrinks })
	foods := slices.DeleteFunc(slices.Clone(products), func(m menuProduct) bool { return !isFood(m, p.FoodCategories) })
	drinkLines, drinkTotal, drinksMissing := cheapest(drinks, stock, p.People)
	foodLines, foodTotal, foodsMissing := cheapest(foods, stock, p.People)
	if drinksMissing > 0 || foodsMissing > 0 {
		return say(ctx, emit, fmt.Sprintf("No hay productos suficientes con stock para %d personas: faltan %d bebidas y %d alimentos de los que prefieres.",
			p.People, drinksMissing, foodsMissing))
	}
	base := comboPlan{Lines: append(drinkLines, foodLines...), Total: drinkTotal + foodTotal}

	// Si ni el combo más barato cabe, se dice al instante cuál es el mínimo.
	if base.Total > p.Budget {
		return say(ctx, emit, fmt.Sprintf("Con %s no alcanza para %d personas. El combo más económico posible es:\n%sTotal: %s (te faltan %s).",
			pesos(p.Budget), p.People, describe(base), pesos(base.Total), pesos(base.Total-p.Budget)))
	}

	if err := setJSON(ctx, comboParamsKey, p); err != nil {
		return "", err
	}
	if err := setJSON(ctx, comboBaseKey, base); err != nil {
		return "", err
	}

	margin := p.Budget - base.Total
	if margin < minUpgrade(products, stock, base, p.FoodCategories) {
		return say(ctx, emit, baseMessage(p, base)+"\n\n¿Te lo pido?")
	}

	// Hay margen: el bucle busca una mejora. Su entrada describe el combo base.
	event := session.NewEvent(ctx, ctx.InvocationID())
	event.Routes = []string{routeUpsell}
	if err := emit(event); err != nil {
		return "", err
	}
	request := fmt.Sprintf("Combo base para %d personas (presupuesto %s, margen %s):\n%sTotal: %s.",
		p.People, pesos(p.Budget), pesos(margin), describe(base), pesos(base.Total))
	if p.Notes != "" {
		request += " Preferencias del cliente: " + p.Notes + "."
	}
	return request, nil
}

func baseMessage(p comboParams, base comboPlan) string {
	return fmt.Sprintf("Te propongo este combo para %d personas:\n%sTotal: %s (tu presupuesto: %s).",
		p.People, describe(base), pesos(base.Total), pesos(p.Budget))
}

type CheckUpsellRequest struct {
	Items []ComboItem `json:"items" jsonschema:"Lista COMPLETA de productos del combo mejorado"`
}

type CheckUpsellResponse struct {
	Lines    []ComboLine `json:"lines"`
	Total    int         `json:"total"`
	Extra    int         `json:"extra" jsonschema:"cuánto más cuesta que el combo base"`
	Problems []string    `json:"problems"`
	Approved bool        `json:"approved"`
}

// CheckUpsell verifica en código una mejora propuesta por el modelo. Las personas, el presupuesto y
// el combo base los lee del estado: el modelo no puede cambiarlos para que la mejora "cuadre".
func (c combos) CheckUpsell(ctx agent.Context, req CheckUpsellRequest) (CheckUpsellResponse, error) {
	var p comboParams
	var base comboPlan
	if getJSON(ctx, comboParamsKey, &p) != nil || getJSON(ctx, comboBaseKey, &base) != nil {
		return CheckUpsellResponse{}, fmt.Errorf("no hay un combo base para mejorar")
	}
	products, stock, err := c.catalog(ctx)
	if err != nil {
		return CheckUpsellResponse{}, err
	}

	response := CheckUpsellResponse{Problems: []string{}}
	drinks, foods := 0, 0
	for _, item := range req.Items {
		i := slices.IndexFunc(products, func(m menuProduct) bool { return fold(m.Name) == fold(strings.TrimSpace(item.Product)) })
		if i < 0 {
			response.Problems = append(response.Problems, fmt.Sprintf("%q no está en el menú", item.Product))
			continue
		}
		m := products[i]
		if item.Quantity <= 0 {
			response.Problems = append(response.Problems, fmt.Sprintf("la cantidad de %s debe ser mayor a cero", m.Name))
			continue
		}
		if item.Quantity > stock[m.ID] {
			response.Problems = append(response.Problems, fmt.Sprintf("%s: pides %d y solo hay %d", m.Name, item.Quantity, stock[m.ID]))
		}
		switch {
		case m.Category == categoryDrinks:
			drinks += item.Quantity
		case isFood(m, p.FoodCategories):
			foods += item.Quantity
		default:
			response.Problems = append(response.Problems, fmt.Sprintf("%s no es de lo que prefiere el cliente", m.Name))
		}
		response.Lines = append(response.Lines, ComboLine{Product: m.Name, Quantity: item.Quantity, UnitPrice: m.Price, Subtotal: item.Quantity * m.Price})
		response.Total += item.Quantity * m.Price
	}
	if drinks < p.People {
		response.Problems = append(response.Problems, fmt.Sprintf("hay %d bebidas para %d personas", drinks, p.People))
	}
	if foods < p.People {
		response.Problems = append(response.Problems, fmt.Sprintf("hay %d alimentos para %d personas", foods, p.People))
	}
	if response.Total > p.Budget {
		response.Problems = append(response.Problems, fmt.Sprintf("se pasa del presupuesto por %s", pesos(response.Total-p.Budget)))
	}
	if response.Total <= base.Total {
		response.Problems = append(response.Problems, "no es una mejora: no cuesta más que el combo base")
	}
	response.Extra = response.Total - base.Total
	response.Approved = len(response.Problems) == 0

	if err := setJSON(ctx, upsellCheckKey, response); err != nil {
		return CheckUpsellResponse{}, err
	}
	// La salida del bucle la decide el código: si la mejora cumple, Escalate lo detiene.
	if response.Approved {
		ctx.Actions().Escalate = true
	}
	return response, nil
}

// comboResult es el nodo que cierra el camino del combo: el combo base y, si el bucle encontró una
// mejora que cumple, la opción mejorada. La entrada se ignora: un agente compuesto, como el bucle,
// no entrega su texto como salida del nodo.
func comboResult(ctx agent.Context, _ any, emit func(*session.Event) error) (string, error) {
	var p comboParams
	var base comboPlan
	if getJSON(ctx, comboParamsKey, &p) != nil || getJSON(ctx, comboBaseKey, &base) != nil {
		return say(ctx, emit, "No pude armar el combo. Cuéntame de nuevo para cuántas personas es y tu presupuesto.")
	}
	msg := baseMessage(p, base)
	var upsell CheckUpsellResponse
	if getJSON(ctx, upsellCheckKey, &upsell) == nil && upsell.Approved {
		msg += fmt.Sprintf("\n\nSi quieres algo mejor, por %s más (total %s):\n%s\n¿Cuál prefieres?",
			pesos(upsell.Extra), pesos(upsell.Total), strings.TrimSuffix(describe(comboPlan{Lines: upsell.Lines}), "\n"))
	} else {
		msg += "\n\n¿Te lo pido?"
	}
	return say(ctx, emit, msg)
}

// upsellLoop busca, en bucle, una mejora del combo base que el código apruebe.
func upsellLoop(m model.LLM, cafeTools tool.Toolset, c combos) (agent.Agent, error) {
	plannerConfig := llmagent.Config{
		Name:        "upsell_planner",
		Model:       m,
		Description: "Propone una mejora atractiva de un combo.",
		Instruction: `
			Eres el encargado de ventas de una cafetería. Recibes un combo base que ya
			cumple el presupuesto del cliente y el margen que queda. Propón UNA mejora
			atractiva que el cliente pueda preferir: cambiar algunos productos por otros
			mejores, o agregar algo para compartir. Consulta "list_menu" para ver los
			productos y sus precios. Mantén al menos una bebida y un alimento por persona,
			respeta las preferencias del cliente y no te pases del presupuesto.
			Correcciones del cajero sobre tu propuesta anterior: {upsell_feedback?}
			Responde solo con la lista COMPLETA del combo mejorado, una línea por producto
			con su cantidad.
		`,
		Toolsets:  []tool.Toolset{tool.FilterToolset(cafeTools, tool.AllowedToolsPredicate([]string{"list_menu"}))},
		OutputKey: "upsell",
	}
	planner, err := llmagent.New(plannerConfig)
	if err != nil {
		return nil, err
	}

	checkUpsell, err := functiontool.New(functiontool.Config{
		Name:        "check_upsell",
		Description: "Verifica con los precios y el stock reales que una mejora del combo cumpla: presupuesto, stock, una bebida y un alimento por persona, y que cueste más que el combo base.",
	}, c.CheckUpsell)
	if err != nil {
		return nil, err
	}
	checkerConfig := llmagent.Config{
		Name:        "upsell_checker",
		Model:       m,
		Description: "Verifica que la mejora del combo cumpla.",
		Instruction: `
			Eres el cajero de una cafetería. Verifica esta mejora del combo:
			{upsell}
			Usa siempre la herramienta "check_upsell" con la lista completa de productos.
			- Si approved es true, responde "Mejora aprobada".
			- Si no, explica los problemas (problems) para que la ajusten.
		`,
		Tools:     []tool.Tool{checkUpsell},
		OutputKey: "upsell_feedback",
	}
	checker, err := llmagent.New(checkerConfig)
	if err != nil {
		return nil, err
	}

	loopConfig := loopagent.Config{
		AgentConfig: agent.Config{
			Name:        "upsell_loop",
			Description: "Busca una mejora del combo base que cumpla el presupuesto.",
			SubAgents:   []agent.Agent{planner, checker},
		},
		// Límite de seguridad: si no encuentra una mejora en 3 vueltas, se ofrece solo el combo base.
		MaxIterations: 3,
	}
	return loopagent.New(loopConfig)
}
