package main

import (
	"fmt"
	"strconv"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// offTopicKey cuenta, en el estado, cuántas veces el cliente ha pedido algo ajeno al pedido en curso.
const offTopicKey = "barista_off_topic"

// offTopicRequestKey guarda lo primero que pidió el cliente (con su comentario, por ejemplo), para no
// perderlo cuando insiste con un "no, quiero dejar la reseña ahora".
const offTopicRequestKey = "barista_off_topic_request"

// off_topic decide en código qué hace el barista cuando el cliente pide algo que no es su pedido:
// la primera vez lo aplaza con cortesía y, si el cliente insiste, termina la tarea. Contar la
// insistencia es cosa del código: dejada al modelo, aplazaba una y otra vez sin terminar nunca.
type OffTopicRequest struct {
	Request string `json:"request" jsonschema:"lo que pidió el cliente que no es el pedido (por ejemplo: dejar una reseña)"`
}

type OffTopicResponse struct {
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
}

func offTopic(ctx agent.Context, req OffTopicRequest) (OffTopicResponse, error) {
	count := 0
	if v, err := ctx.State().Get(offTopicKey); err == nil {
		count, _ = strconv.Atoi(fmt.Sprint(v)) // el estado guardado puede devolver 1 o 1.0
	}
	count++

	// Primera vez: se aplaza con cortesía.
	if count == 1 {
		if err := ctx.State().Set(offTopicKey, count); err != nil {
			return OffTopicResponse{}, err
		}
		if err := ctx.State().Set(offTopicRequestKey, req.Request); err != nil {
			return OffTopicResponse{}, err
		}
		return OffTopicResponse{
			Action: "aplazar",
			Instruction: "Dile con cortesía que primero terminan el pedido y pregúntale si lo continúan o lo cancelan. " +
				"No hagas nada con lo que pidió.",
		}, nil
	}

	// Insiste: se reinicia la cuenta y se termina la tarea con lo primero que pidió.
	pending := req.Request
	if first, err := ctx.State().Get(offTopicRequestKey); err == nil && fmt.Sprint(first) != "" {
		pending = fmt.Sprint(first)
	}
	if err := ctx.State().Set(offTopicKey, 0); err != nil {
		return OffTopicResponse{}, err
	}
	return OffTopicResponse{
		Action: "terminar",
		Instruction: fmt.Sprintf("Termina la tarea AHORA con finish_task: order_id 0, total 0, summary \"pedido sin terminar\" "+
			"y pending_request %q.", pending),
	}, nil
}

func offTopicTool() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "off_topic",
		Description: "Úsala cada vez que el cliente pida algo que no es su pedido (dejar una reseña, ver un reporte, cambiar el inventario...). Te dice qué hacer: aplazarlo o terminar la tarea.",
	}, offTopic)
}
