#!/usr/bin/env bash
# Simula el sistema del supervisor: le habla al agente por la API REST del ADK.
# Requiere el agente corriendo con: go run . web api -include_debug_api webui
#
#   ./supervisor.sh pedir "Aplica el cupón ESPECIAL-50 a las zapatillas"
#   ./supervisor.sh aprobar        # aprueba con el descuento pedido
#   ./supervisor.sh aprobar 20     # aprueba, pero con 20 %
#   ./supervisor.sh rechazar
set -euo pipefail

API=http://localhost:8080/api
APP=store_agent
USER_ID=supervisor

# Imprime los textos del agente y, si pide aprobación, su mensaje.
mostrar() {
	jq -r '.[] | .content.parts[]? |
		if .text then "Agente -> " + .text
		elif .functionCall.name == "adk_request_confirmation" then
			"Aprobación pendiente -> " + .functionCall.args.toolConfirmation.hint
		else empty end'
}

run() {
	curl -s -X POST "$API/run" -H 'Content-Type: application/json' -d "$1"
}

case "${1:-}" in
pedir)
	SESSION="demo-$(date +%s)"
	echo "$SESSION" > .sesion
	curl -s -X POST "$API/apps/$APP/users/$USER_ID/sessions/$SESSION" -H 'Content-Type: application/json' -d '{}' > /dev/null
	RESP=$(run "$(jq -n --arg app "$APP" --arg user "$USER_ID" --arg s "$SESSION" --arg text "$2" \
		'{appName: $app, userId: $user, sessionId: $s, newMessage: {role: "user", parts: [{text: $text}]}}')")
	echo "$RESP" | jq -r '[.[] | .content.parts[]? | .functionCall? | select(.name == "adk_request_confirmation") | .id][0] // empty' > .pendiente
	echo "$RESP" | mostrar
	;;
aprobar | rechazar)
	SESSION=$(cat .sesion)
	ID=$(cat .pendiente)
	if [ -z "$ID" ]; then
		echo "No hay ninguna aprobación pendiente."
		exit 1
	fi
	if [ "$1" = "rechazar" ]; then
		RESPONSE='{"confirmed": false}'
	elif [ -n "${2:-}" ]; then
		RESPONSE=$(jq -n --argjson d "$2" '{confirmed: true, payload: {discount: $d}}')
	else
		RESPONSE='{"confirmed": true}'
	fi
	run "$(jq -n --arg app "$APP" --arg user "$USER_ID" --arg s "$SESSION" --arg id "$ID" --argjson r "$RESPONSE" \
		'{appName: $app, userId: $user, sessionId: $s, newMessage: {role: "user", parts: [{functionResponse: {id: $id, name: "adk_request_confirmation", response: $r}}]}}')" | mostrar
	: > .pendiente
	;;
*)
	echo "Uso: $0 pedir \"mensaje\" | aprobar [descuento] | rechazar"
	exit 1
	;;
esac
