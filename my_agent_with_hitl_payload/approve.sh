#!/usr/bin/env bash
# Aprueba (con un descuento modificado) o rechaza la confirmación pendiente
# de una sesión, usando la API REST del ADK.
#
# Uso:
#   ./approve.sh <session_id> <descuento>   # aprueba con ese descuento
#   ./approve.sh <session_id> rechazar      # rechaza el cupón
set -euo pipefail

SESSION_ID=${1:?Falta el session_id}
DECISION=${2:?Falta el descuento o la palabra rechazar}

API=${API:-http://localhost:8080/api}
APP_NAME=${APP_NAME:-store_agent}
USER_ID=${USER_ID:-user}

if [ "$DECISION" = "rechazar" ]; then
  RESPONSE='{"confirmed": false}'
else
  RESPONSE="{\"confirmed\": true, \"payload\": {\"discount\": $DECISION}}"
fi

# ID de la última llamada adk_request_confirmation de la sesión
CALL_ID=$(curl -s "$API/apps/$APP_NAME/users/$USER_ID/sessions/$SESSION_ID" |
  jq -r '[.events[].content.parts[]?.functionCall | select(.name == "adk_request_confirmation") | .id] | last')

if [ "$CALL_ID" = "null" ]; then
  echo "No hay confirmaciones pendientes en la sesión $SESSION_ID"
  exit 1
fi

curl -s "$API/run" \
  -H "Content-Type: application/json" \
  -d "{
    \"appName\": \"$APP_NAME\",
    \"userId\": \"$USER_ID\",
    \"sessionId\": \"$SESSION_ID\",
    \"newMessage\": {
      \"role\": \"user\",
      \"parts\": [{
        \"functionResponse\": {
          \"id\": \"$CALL_ID\",
          \"name\": \"adk_request_confirmation\",
          \"response\": $RESPONSE
        }
      }]
    }
  }" | jq -r '.[].content.parts[]?.text // empty'
