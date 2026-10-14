package main

import (
	"context"
	"fmt"
	"os"

	"github.com/glebarez/sqlite"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/session/database"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
	"google.golang.org/genai"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	ctx := context.Background()

	// ----------------------------------------------
	// Creación del modelo
	// ----------------------------------------------
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		fmt.Println("Debes configurar GOOGLE_API_KEY")
		os.Exit(1)
	}

	modelName := "gemini-3.8-flash"

	clientConfig := genai.ClientConfig{
		APIKey: apiKey,
	}

	model, err := gemini.NewModel(ctx, modelName, &clientConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Tool que guarda datos en el estado de la sesión
	// ----------------------------------------------
	saveFavoriteDrinkToolConfig := functiontool.Config{
		Name:        "save_favorite_drink",
		Description: "Guarda la bebida favorita del cliente para recordarla en futuras visitas.",
	}
	saveFavoriteDrinkTool, err := functiontool.New(saveFavoriteDrinkToolConfig, SaveFavoriteDrink)
	if err != nil {
		fmt.Printf("Hubo un error al convertir la función %s a tool", err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------
	// Creación del agente
	// ----------------------------------------------
	agentConfig := llmagent.Config{
		Name:        "coffee_agent",
		Model:       model,
		Description: "Atiendes a los clientes de una cafetería y recuerdas su bebida favorita",
		// {user:favorite_drink?} se reemplaza con el valor guardado en el estado.
		// El "?" evita un error cuando la clave todavía no existe.
		Instruction: `
			Eres el barista de una cafetería. Cuando el cliente te diga cuál es su
			bebida favorita, guárdala con la herramienta "save_favorite_drink".

			La bebida favorita del cliente es: {user:favorite_drink?}

			Si el cliente pide "lo de siempre", prepárale su bebida favorita.
			Si no conoces su bebida favorita, pregúntale cuál es.
		`,
		Tools: []tool.Tool{
			saveFavoriteDrinkTool,
		},
	}

	a, err := llmagent.New(agentConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Servicio de sesiones en base de datos (SQLite)
	// Las sesiones y su estado se guardan en el archivo sessions.db,
	// así que sobreviven a un reinicio del servidor.
	// ----------------------------------------------
	// El logger en modo silencioso evita que GORM imprima cada consulta SQL
	gormConfig := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	sessionService, err := database.NewSessionService(sqlite.Open("sessions.db"), gormConfig)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// Crea las tablas que necesita el ADK si todavía no existen
	err = database.AutoMigrate(sessionService)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	// ----------------------------------------------
	// Lanzar el agente
	// ----------------------------------------------
	launcherConfig := launcher.Config{
		AgentLoader:    agent.NewSingleLoader(a),
		SessionService: sessionService,
	}

	launch := full.NewLauncher()
	err = launch.Execute(ctx, &launcherConfig, os.Args[1:])
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
