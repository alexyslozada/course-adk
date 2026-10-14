package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/agent/workflowagent"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

// Rutas del grafo.
const (
	routeCombo  = "combo"
	routeQuery  = "consulta"
	routeUpsell = "mejora" // hay margen en el presupuesto: se busca una mejora del combo base
)

// comboParams son los datos que se extraen de la conversación para armar un combo.
type comboParams struct {
	People         int      `json:"people"`
	Budget         int      `json:"budget"`
	FoodCategories []string `json:"food_categories"`
	Notes          string   `json:"notes"`
}

// decodeNodeInput lee la entrada de un nodo en dst. Un agente con OutputSchema entrega su salida ya
// convertida en objeto, y a veces como el texto JSON: se aceptan las dos formas.
func decodeNodeInput(input any, dst any) error {
	if text, ok := input.(string); ok {
		return json.Unmarshal([]byte(text), dst)
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// route decide el camino del grafo según la clasificación del LLM. Es código Go, no un LLM:
// recibe la salida del clasificador, emite la ruta y pasa el mensaje original al siguiente nodo.
func route(ctx agent.Context, input any, emit func(*session.Event) error) (string, error) {
	kind := strings.ToLower(strings.Trim(strings.TrimSpace(fmt.Sprint(input)), ".\"'"))

	event := session.NewEvent(ctx, ctx.InvocationID())
	event.Routes = []string{kind}
	if err := emit(event); err != nil {
		return "", err
	}
	return userMessage(ctx), nil
}

// say emite un mensaje de texto como respuesta del flujo. La salida de un nodo Go (Event.Output) la
// leen los nodos siguientes, pero agenttool solo devuelve el texto del último evento con contenido:
// un nodo que cierra el grafo debe emitirlo.
func say(ctx agent.Context, emit func(*session.Event) error, text string) (string, error) {
	event := session.NewEvent(ctx, ctx.InvocationID())
	event.LLMResponse.Content = genai.NewContentFromText(text, genai.RoleModel)
	if err := emit(event); err != nil {
		return "", err
	}
	return text, nil
}

// help es el camino por defecto: una respuesta fija, sin LLM.
func help(ctx agent.Context, _ any, emit func(*session.Event) error) (string, error) {
	return say(ctx, emit, "Puedo ayudarte con tu pedido (dime qué quieres y lo registro), con precios y "+
		"disponibilidad de productos, y a armar un combo según tu presupuesto "+
		"(por ejemplo: un combo para 4 personas con máximo $50.000).")
}

// userMessage devuelve el texto que escribió el usuario.
func userMessage(ctx agent.Context) string {
	content := ctx.UserContent()
	if content == nil {
		return ""
	}
	var text strings.Builder
	for _, part := range content.Parts {
		text.WriteString(part.Text)
	}
	return strings.TrimSpace(text.String())
}

// customerFlow arma el grafo que clasifica lo que quiere el cliente y lo manda por su camino:
//
//	clasificador → router ─ combo ───→ extractor → combo base en Go ─ mejora → bucle que busca una mejora → respuesta en Go
//	                                                  (si faltan datos, no alcanza o no hay margen, responde ahí mismo)
//	                       ─ consulta → LLM que cotiza con el menú y el stock
//	                       ─ otro ────→ ayuda fija (Go)
//
// y lo devuelve como una tool para que el coordinador lo use.
func customerFlow(m model.LLM, cafeTools tool.Toolset, app *appclient.Client) (tool.Tool, error) {
	// ----------------------------------------------
	// Agentes (LLM)
	// ----------------------------------------------
	classifierConfig := llmagent.Config{
		Name:        "request_classifier",
		Model:       m,
		Description: "Clasifica lo que quiere un cliente.",
		Instruction: `
			Recibes la conversación de un cliente de una cafetería y lo que pide ahora.
			Clasifica lo que quiere AHORA, teniendo en cuenta la conversación. Responde
			exactamente una palabra, en minúscula y sin puntuación:
			- combo: quiere armar o cotizar algo para varias personas, con o sin
			  presupuesto, o ajustar un combo que ya venían armando (cambiar el
			  presupuesto, las personas o las preferencias).
			- consulta: pregunta por precios, disponibilidad o qué hay en el menú.
			- otro: cualquier otra cosa.
		`,
	}
	classifier, err := llmagent.New(classifierConfig)
	if err != nil {
		return nil, err
	}

	quoterConfig := llmagent.Config{
		Name:        "quoter",
		Model:       m,
		Description: "Responde consultas de precios y disponibilidad.",
		Instruction: `
			Eres el barista de una cafetería y respondes consultas de precios y
			disponibilidad. Recibes la conversación y lo que pide el cliente ahora. Usa "list_menu" para los precios y "get_stock" para saber
			si hay. Si piden varios productos, da el precio de cada uno y el total.
			Nunca inventes productos, precios ni cantidades. Habla en español, breve y
			en texto plano. Esto es solo una consulta: no registres ninguna venta; si
			el cliente quiere pedir, dile que te lo indique y lo registras.
		`,
		Toolsets: []tool.Toolset{tool.FilterToolset(cafeTools, tool.AllowedToolsPredicate([]string{"list_menu", "get_stock"}))},
	}
	quoter, err := llmagent.New(quoterConfig)
	if err != nil {
		return nil, err
	}

	extractorConfig := llmagent.Config{
		Name:        "combo_params",
		Model:       m,
		Description: "Extrae personas, presupuesto y preferencias de un combo.",
		Instruction: `
			Recibes la conversación de un cliente de una cafetería y lo que pide ahora.
			Extrae los datos del combo que quiere: número de personas, presupuesto
			máximo en pesos (como número entero, por ejemplo 48.000 es 48000) y sus
			preferencias. Usa el valor más reciente que dio el cliente. Si no ha dicho
			las personas o el presupuesto, déjalos en 0: nunca los inventes.
			En food_categories pon las categorías de alimento que prefiere, solo si lo
			dijo: "carbohidratos" o "algo de sal" son Panadería y Comida; "algo dulce"
			es Postres; "sin dulce" es Panadería y Comida. Si no dijo nada, déjalo vacío.
		`,
		OutputSchema: &comboParamsSchema,
	}
	extractor, err := llmagent.New(extractorConfig)
	if err != nil {
		return nil, err
	}

	c := combos{app: app}
	upsell, err := upsellLoop(m, cafeTools, c)
	if err != nil {
		return nil, err
	}

	// ----------------------------------------------
	// Nodos del grafo: AgentNode envuelve un agente; FunctionNode, una función Go.
	// El bucle de la mejora también es un agente, así que entra como un nodo más.
	// ----------------------------------------------
	classifierNode, err := workflow.NewAgentNode(classifier, workflow.NodeConfig{})
	if err != nil {
		return nil, err
	}
	extractorNode, err := workflow.NewAgentNode(extractor, workflow.NodeConfig{})
	if err != nil {
		return nil, err
	}
	upsellNode, err := workflow.NewAgentNode(upsell, workflow.NodeConfig{})
	if err != nil {
		return nil, err
	}
	quoterNode, err := workflow.NewAgentNode(quoter, workflow.NodeConfig{})
	if err != nil {
		return nil, err
	}
	routerNode := workflow.NewEmittingFunctionNode("router", route, workflow.NodeConfig{})
	builderNode := workflow.NewEmittingFunctionNode("combo_builder", c.Build, workflow.NodeConfig{})
	comboResultNode := workflow.NewEmittingFunctionNode("combo_result", comboResult, workflow.NodeConfig{})
	helpNode := workflow.NewEmittingFunctionNode("help", help, workflow.NodeConfig{})

	// ----------------------------------------------
	// Aristas: las que tienen Route solo se siguen si el router emitió esa ruta;
	// Default se sigue cuando ninguna otra coincide.
	// ----------------------------------------------
	edges := workflow.Concat(
		workflow.Chain(workflow.Start, classifierNode, routerNode),
		[]workflow.Edge{
			{From: routerNode, To: extractorNode, Route: workflow.StringRoute(routeCombo)},
			{From: extractorNode, To: builderNode},
			{From: builderNode, To: upsellNode, Route: workflow.StringRoute(routeUpsell)},
			{From: routerNode, To: quoterNode, Route: workflow.StringRoute(routeQuery)},
			{From: routerNode, To: helpNode, Route: workflow.Default},
		},
		workflow.Chain(upsellNode, comboResultNode),
	)
	graphConfig := workflowagent.Config{
		Name:        "customer_flow",
		Description: "Atiende lo que pide un cliente: arma combos con presupuesto, responde consultas de precios y disponibilidad, y da ayuda general.",
		Edges:       edges,
		SubAgents:   []agent.Agent{classifier, extractor, upsell, quoter},
	}
	graph, err := workflowagent.New(graphConfig)
	if err != nil {
		return nil, err
	}
	return conversationTool(graph.Name(), graph)
}
