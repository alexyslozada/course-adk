// Sistema de agentes de la cafetería (ADK): un coordinador que delega en el barista y el administrador.
package main

import (
	"context"
	"fmt"
	"os"

	"alexyslozada/course-adk/cafe_app/internal/appclient"
	"github.com/glebarez/sqlite"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/session/database"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/genai"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config agrupa lo que se lee del entorno.
type Config struct {
	GoogleAPIKey string
	// El barista usa un modelo de Anthropic: con Gemini flash-lite leía mal el menú
	// ("no tenemos latte") y decidía mal.
	AnthropicAPIKey string
	BaristaModel    string
	GeminiModel     string
	MCPURL          string
	MCPToken        string
	SessionsDB      string           // archivo SQLite donde se guardan las conversaciones
	BakeryURL       string           // la panadería externa, por A2A
	App             appclient.Config // la API de la app, para las tools Go del administrador
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() (Config, error) {
	config := Config{
		GoogleAPIKey:    os.Getenv("GOOGLE_API_KEY"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		BaristaModel:    getenv("CAFE_BARISTA_MODEL", "claude-haiku-4-5"),
		GeminiModel:     getenv("CAFE_GEMINI_MODEL", "gemini-3.8-flash"),
		MCPURL:          os.Getenv("CAFE_MCP_URL"),
		MCPToken:        os.Getenv("CAFE_MCP_TOKEN"),
		SessionsDB:      getenv("CAFE_SESSIONS_DB", "sessions.db"),
		BakeryURL:       getenv("CAFE_BAKERY_URL", "http://localhost:8001"),
		App: appclient.Config{
			AppURL:      getenv("CAFE_APP_URL", "http://localhost:8080"),
			AppUser:     getenv("CAFE_APP_USER", "asistente"),
			AppPassword: getenv("CAFE_APP_PASSWORD", "asistente123"),
		},
	}
	if config.MCPURL == "" {
		config.MCPURL = "http://localhost:8090"
	}
	if config.GoogleAPIKey == "" {
		return config, fmt.Errorf("debes configurar GOOGLE_API_KEY")
	}
	if config.AnthropicAPIKey == "" {
		return config, fmt.Errorf("debes configurar ANTHROPIC_API_KEY (el barista usa un modelo de Anthropic)")
	}
	if config.MCPToken == "" {
		return config, fmt.Errorf("debes configurar CAFE_MCP_TOKEN (el mismo del servidor MCP)")
	}
	return config, nil
}

func main() {
	ctx := context.Background()

	config, err := loadConfig()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del modelo
	// ----------------------------------------------
	modelName := config.GeminiModel

	clientConfig := genai.ClientConfig{
		APIKey:     config.GoogleAPIKey,
		HTTPClient: modelHTTPClient(os.Getenv("CAFE_LOG_MODEL_CALLS") != ""),
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Modelo del barista: Claude Haiku de Anthropic, por el endpoint compatible con OpenAI.
	baristaLLM, err := newAnthropicModel(ctx, config.AnthropicAPIKey, config.BaristaModel)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// MCP de la cafetería (con token)
	// ----------------------------------------------
	cafeTools, err := cafeMCP(config.MCPURL, config.MCPToken)
	if err != nil {
		fmt.Println("No se pudo cargar el MCP de la cafetería:", err)
		os.Exit(1)
	}

	// Cliente de la API de la app, para las tools Go (barista, administrador y flujos).
	appAPI := appclient.New(config.App)

	registerSale, err := order{app: appAPI}.tool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	offTopic, err := offTopicTool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	// Memoria entre conversaciones: preferencias del usuario (user:) y avisos para todos (app:).
	savePreferences, err := preferencesTool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	saveNotices, err := noticesTool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Barista en modo task
	// Conversa con el cliente hasta tener el pedido completo y lo registra con register_sale,
	// una tool Go que pide la confirmación del cliente con una tarjeta (HITL). Devuelve su
	// OutputSchema al coordinador con finish_task.
	// ----------------------------------------------
	baristaConfig := llmagent.Config{
		Name:        "barista",
		Model:       baristaLLM,
		Description: "Toma el pedido de un cliente de la cafetería y lo registra.",
		Instruction: `
			Eres el barista de la cafetería. Tomas el pedido del cliente y lo
			registras. Habla siempre en español, de forma amable y breve, en texto
			plano (sin Markdown: nada de asteriscos ni listas con guiones).
			No narres lo que vas a hacer ("déjame consultar el menú"): llama a las
			herramientas directamente y escribe solo lo que el cliente necesita leer.
			Háblale al cliente de tú ("tu pedido", "quieres"). La solicitud que recibes
			puede venir escrita en tercera persona ("el cliente quiere..."): es una
			nota interna, nunca se la repitas así al cliente.
			Avisos de la cafetería para todo el equipo: {app:avisos?}
			Si un aviso afecta el pedido, menciónalo.

			Pasos:
			1. Consulta el menú con "list_menu". Nunca inventes productos ni precios.
			2. Pregunta lo que falte: qué productos y cuántas unidades. Usa
			   exactamente las cantidades que te piden: no las confundas con el número
			   de personas. Si pide algo que no está en el menú, dilo y ofrece
			   alternativas del menú.
			3. Necesitas su nombre ANTES de registrar. Nombre que el cliente ya te dio en
			   esta conversación: {customer_name?}. Si ya lo tienes, úsalo sin volver a
			   preguntar (si el cliente da otro nombre, usa el nuevo). Si no lo tienes,
			   pregúntaselo y usa el nombre que te dé, tal cual. Solo si dice que no
			   quiere darlo, usa "anónimo"; nunca lo decidas por tu cuenta. La
			   herramienta comprueba que el cliente lo dijo.
			4. Cuando tengas productos, cantidades y nombre, llama a "register_sale".
			   La herramienta calcula el total, revisa el stock y le muestra al cliente
			   una tarjeta para que apruebe o rechace el pedido: no pidas confirmación
			   en el chat. No hay otra forma de registrar una venta.
			5. Si "register_sale" dice que no se puede registrar (por ejemplo, por falta
			   de stock), explícale los problemas y ofrece cambiar el pedido. No compares
			   cantidades con el inventario por tu cuenta: la herramienta lo hace.
			6. Mientras la aprobación esté pendiente, espera. Si el cliente aprueba, termina
			   la tarea con el número de la venta, el total y el resumen.
			7. Si el cliente pide algo que no es el pedido (armar o cotizar un combo,
			   pedir opciones según un presupuesto, dejar una reseña, ver un reporte,
			   cambiar el inventario...), llama a "off_topic" con lo que pidió
			   y haz exactamente lo que te indique. No decidas tú si aplazarlo o
			   terminar. Nunca armes combos ni opciones de precios por tu cuenta: los
			   calcula otro agente con los precios y el stock reales. Vuelve a llamarla en CADA mensaje del cliente que no continúe
			   el pedido (que no hable de productos, cantidades, su nombre o la
			   aprobación), por ejemplo si insiste en lo mismo o responde "no, quiero
			   lo otro" cuando le preguntaste si continúan. Nunca respondas tú sobre
			   lo ajeno ni se lo vuelvas a aplazar por tu cuenta. Cuando la herramienta te
			   diga "terminar", llama a finish_task sin escribirle nada al cliente.
			8. Si "register_sale" devuelve un error que dice "rejected" (rechazada), el
			   cliente canceló el pedido: NO vuelvas a llamarla. Termina la tarea con
			   order_id 0, total 0 y el resumen "pedido cancelado".
		`,
		Mode:         llmagent.ModeTask,
		OutputSchema: &orderSchema,
		// Una tarea no puede transferir la conversación: debe terminar con finish_task.
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
		// Solo lecturas del MCP: crear la venta es cosa de register_sale.
		Toolsets: []tool.Toolset{tool.FilterToolset(cafeTools, tool.AllowedToolsPredicate([]string{"list_menu", "get_stock"}))},
		Tools:    []tool.Tool{registerSale, offTopic},
		// El barista habla con el cliente sin pasar por el coordinador: también guarda la conversación
		// en el estado, para que register_sale compruebe que el cliente dijo su nombre.
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{rememberConversation},
	}
	barista, err := llmagent.New(baristaConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Administrador en modo task
	// Sus tools son funciones Go que llaman a la API de la app. Las que cambian
	// datos piden confirmación humana (HITL): el ADK pausa hasta que alguien responde.
	// ----------------------------------------------
	adminTools, err := admin{app: appAPI}.tools()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	// La panadería es un agente remoto (A2A): el administrador le pide pan con order_bread.
	bakery, err := newBakeryAgent(config.BakeryURL)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	orderBread, err := restock{app: appAPI, bakery: bakery}.tool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	administratorConfig := llmagent.Config{
		Name:        "administrador",
		Model:       model,
		Description: "Administra la cafetería: consulta y ajusta el inventario, le pide pan a la panadería y aplica descuentos a ventas.",
		Instruction: `
			Eres el administrador de la cafetería. Habla siempre en español, de forma
			breve, en texto plano (sin Markdown).

			- Para ver el inventario usa "get_inventory". Nunca inventes cantidades.
			- Para cambiar el inventario usa "adjust_inventory" con la cantidad total
			  que debe quedar. Si el usuario da una diferencia ("llegaron 30"), suma
			  esa diferencia a la cantidad actual.
			- Para ver una venta (productos, descuento y lo que debe pagar el cliente)
			  usa "get_sale".
			- Para un descuento a una venta usa "approve_discount". El número de venta
			  y el porcentaje deben venir del usuario; si falta alguno, pregúntalo.
			- Para publicar, cambiar o quitar un aviso para todo el equipo (por ejemplo,
			  "avisa que hoy no hay pan de queso") usa "save_notices" con la lista
			  completa de avisos: los actuales más el nuevo, sin el que pidan quitar.
			  Avisos actuales: {app:avisos?}
			- Para pedirle pan a la panadería La Espiga (por ejemplo, "falta pan" o
			  "pide pan de queso") usa "order_bread". Solo se piden productos de
			  panadería; mira en "get_inventory" cuáles están en o bajo su mínimo.
			  Si no dicen cuántas unidades, propón las necesarias para llegar al
			  doble del mínimo: la persona puede cambiar la cantidad en la tarjeta.
			  Si falta más de un pan, pídelos de a uno: el siguiente cuando termine
			  el anterior. El pan llega en el día y la tool lo suma al inventario:
			  no uses "adjust_inventory" después. En el resumen di el número de
			  pedido de la panadería, las unidades, lo que cobra y cómo quedó el
			  inventario.
			- "adjust_inventory", "approve_discount" y "order_bread" piden la
			  confirmación de una persona. Mientras esté
			  pendiente, avísale al usuario que debe aprobar o rechazar y espera.
			  Nunca digas que se hizo un cambio hasta que la tool lo confirme.
			- Termina SIEMPRE llamando a finish_task, también en las consultas: nunca
			  respondas con un texto suelto, porque la tarea queda abierta. El resumen
			  debe traer la respuesta completa para el usuario, con las cifras. Si un
			  cambio se aplica, o el administrador lo rechaza, termina con ese resultado.
			- Si te piden algo que ninguna de tus herramientas puede hacer, termina la
			  tarea de inmediato con status "sin_cambios" y explica en el resumen que
			  no puedes hacerlo. Nunca intentes pasarle la conversación a otro agente.
		`,
		Mode:         llmagent.ModeTask,
		OutputSchema: &adminSchema,
		// Una tarea no puede transferir la conversación: debe terminar con finish_task.
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
		Tools:                    append(adminTools, saveNotices, orderBread),
	}
	administrador, err := llmagent.New(administratorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Reseñas en modo task
	// Habla con el cliente (por eso usa el mismo modelo que el barista), pide la calificación y
	// guarda la reseña con save_review, una tool Go que valida los datos.
	// ----------------------------------------------
	reviewTool, err := reviewer{app: appAPI}.tool()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	reviewerConfig := llmagent.Config{
		Name:        "resenas",
		Model:       baristaLLM,
		Description: "Recoge y guarda la reseña de un cliente sobre la cafetería o un producto.",
		Instruction: `
			Recoges la reseña de un cliente de la cafetería. Habla siempre en español, de
			forma amable y breve, en texto plano (sin Markdown). Háblale de tú. No narres
			lo que vas a hacer: llama a las herramientas directamente. La solicitud que
			recibes puede venir en tercera persona ("el cliente quiere..."): es una nota
			interna, nunca se la repitas así al cliente.

			Pasos:
			1. Necesitas el comentario del cliente y su calificación de 1 a 5. Si no te
			   dio la calificación, pregúntasela: nunca la inventes ni la deduzcas del
			   comentario. Nombre del cliente que ya conoces: {customer_name?}.
			2. Si el comentario es sobre un producto del menú, pásalo en "product".
			3. Si el cliente ya dijo qué opina, ese es el comentario: no le pidas que lo
			   repita ni que lo reescriba. Llama a "save_review" con sus palabras tal cual,
			   sin pedirle confirmación: se guarda directo.
			4. Si "save_review" dice que falta algo o que algo no es válido, pídeselo al
			   cliente.
			5. Cuando se guarde, termina la tarea con status "guardada". Si el cliente
			   desiste, termina con status "cancelada".
			6. Si el cliente pide algo que no es una reseña, termina la tarea con status
			   "cancelada" y dile en el resumen qué pidió.
		`,
		Mode:         llmagent.ModeTask,
		OutputSchema: &reviewTaskSchema,
		Tools:        []tool.Tool{reviewTool},
		// Una tarea no puede transferir la conversación: debe terminar con finish_task.
		DisallowTransferToParent: true,
		DisallowTransferToPeers:  true,
	}
	resenas, err := llmagent.New(reviewerConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Analista: el reporte del día (paralelo + secuencial) como tool del coordinador
	// ----------------------------------------------
	reportTool, err := dailyReport(model, cafeTools)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Flujos: grafo con router y bucle de combos, como tool del coordinador
	// ----------------------------------------------
	flowTool, err := customerFlow(model, cafeTools, appAPI)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Coordinador
	// El barista y el administrador (ModeTask) se vuelven tools del coordinador.
	// ----------------------------------------------
	coordinatorConfig := llmagent.Config{
		Name:        "cafe_coordinator",
		Model:       model,
		Description: "Recibe a los clientes de la cafetería y los atiende.",
		Instruction: `
			Eres el anfitrión de una cafetería. Habla siempre en español, de forma
			amable y breve, en texto plano (sin Markdown: nada de asteriscos ni
			listas con guiones). Háblale al cliente de tú, nunca de usted.
			Tu último mensaje del turno es el único que lee el cliente: cuando un
			agente (barista, administrador, reseñas) termina, incluye en tu respuesta
			el resultado completo.

			Preferencias de este usuario, guardadas en conversaciones anteriores:
			{user:preferencias?}
			Respétalas siempre. Si te pide que recuerdes u olvides algo sobre cómo
			atenderlo ("llámame Juancho", "prefiero respuestas cortas", "olvida
			eso"), llama a "save_preferences" con la lista completa actualizada: las
			actuales más la nueva, sin la que pida olvidar. Luego confírmaselo.

			Avisos de la cafetería para todo el equipo: {app:avisos?}
			Tenlos en cuenta al responder (por ejemplo, si un producto se agotó).

			- Si el cliente quiere pedir algo concreto (bebidas, comida, postres), usa
			  "barista". Cuando termine, confirma el pedido: número de venta, total en
			  pesos y resumen. Si el barista devuelve un pending_request, el cliente
			  dejó el pedido para pedir otra cosa: atiéndelo ahora, como si lo
			  acabara de pedir (por ejemplo, para una reseña usa "resenas"). Si el
			  order_id es 0 y no hay pending_request, el cliente canceló: dile que
			  cancelaste el pedido y que no se registró nada; no hables de problemas
			  ni de errores.
			- Si el cliente quiere armar un combo, o pregunta por precios,
			  disponibilidad o qué hay en el menú, o necesita ayuda general, usa
			  "customer_flow" con lo que pide ahora y transmite su respuesta tal cual
			  (puedes saludar), sin cambiar cifras ni agregar productos.
			  Si después de un combo o una consulta el cliente quiere pedir, usa
			  "barista".
			- Si el cliente quiere dejar una reseña o una opinión, usa "resenas". Cuando
			  termine, dile si quedó guardada; si el status es "cancelada", dile que
			  no se guardó nada, sin hablar de problemas ni pedirle nada más.
			- Si piden ver o cambiar el inventario, ver el detalle o el total de una
			  venta (por ejemplo, "¿en cuánto me quedó la factura?"), o aplicar un
			  descuento a una venta, o publicar o quitar un aviso para todo el equipo,
			  o pedirle pan a la panadería, usa "administrador". Cuando termine, dile al usuario el resultado.
			- Si piden el reporte del día, el resumen del negocio o cómo va el día,
			  llama a "daily_report" de inmediato y responde solo cuando tengas su
			  resultado (no avises antes que lo estás preparando). Preséntalo en texto
			  plano con cada parte en su propia línea, separadas por una línea en
			  blanco: titular, ventas, alertas de inventario, reseñas y
			  recomendaciones (una por línea, numeradas).
			- Si una herramienta falla con un error técnico, dile al cliente que no
			  pudiste hacerlo ahora. Si el administrador rechaza un cambio, dile que
			  no se aplicó y que todo sigue igual. Nunca inventes productos, precios, cantidades ni
			  resultados: si no los tienes de una herramienta, no los digas.
			- Si te saluda, salúdalo y pregúntale en qué le puedes ayudar.
			- Si te pide algo que no tiene que ver con la cafetería, dile con
			  amabilidad que solo puedes ayudar con la cafetería.
		`,
		SubAgents: []agent.Agent{barista, administrador, resenas},
		Tools:     []tool.Tool{reportTool, flowTool, savePreferences},
		// Guarda la conversación en el estado para que los flujos la reciban.
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{rememberConversation},
		// Le pone título a la conversación para la lista de la app.
		BeforeAgentCallbacks: []agent.BeforeAgentCallback{titleConversation},
	}
	coordinator, err := llmagent.New(coordinatorConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// Se lanza solo el coordinador: el barista, el administrador y las reseñas viven dentro de él.
	// ----------------------------------------------
	// Las conversaciones se guardan en SQLite: sobreviven a un reinicio de los agentes.
	gormConfig := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	sessionService, err := database.NewSessionService(sqlite.Open(config.SessionsDB), gormConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Crea las tablas que necesita el ADK si todavía no existen
	if err := database.AutoMigrate(sessionService); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	launcherConfig := launcher.Config{
		AgentLoader:    agent.NewSingleLoader(coordinator),
		SessionService: sessionService,
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
