# Una sola imagen con los cuatro servicios: se compila una vez y cada contenedor arranca su binario
# (ver docker-compose.yml). Las dos bases SQLite (modernc y glebarez) son Go puro: se compila sin CGO
# y la imagen final no necesita sistema operativo (distroless trae solo los certificados para llamar
# a los modelos).
# Los nombres de las imágenes van completos (docker.io/...): Podman no siempre acepta los cortos.

FROM docker.io/library/golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# -p 1 y GOGC=20: un paquete a la vez y un recolector de basura más agresivo en el compilador,
# para que quepa en una máquina con 2 GB de RAM (openai-go es un paquete muy grande).
RUN for service in app mcp agents bakery; do \
      GOGC=20 CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o /out/$service ./$service || exit 1; \
    done \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /out/mcp /out/agents /out/bakery /usr/local/bin/
# /data es donde viven las bases SQLite; el volumen hereda su dueño (el usuario sin privilegios).
COPY --from=build --chown=nonroot:nonroot /out/data /data
WORKDIR /data
ENV CAFE_DB=/data/cafe.db \
    CAFE_SESSIONS_DB=/data/sessions.db
ENTRYPOINT ["/usr/local/bin/app"]
