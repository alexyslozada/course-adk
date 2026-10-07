# cafe_app/app — app base de la cafetería (fase 1)

La aplicación que luego usarán los agentes: SQLite, una API REST en JSON y un frontend con plantillas de Go + htmx. Todavía **no hay agentes**; esta fase se prueba a mano desde el navegador.

```mermaid
graph LR
    N[Navegador] -- "HTML + htmx" --> W[web.go<br/>páginas y fragmentos]
    C[curl / MCP futuro] -- "JSON" --> A[api.go<br/>/api/...]
    W --> S[store.go]
    A --> S
    S --> DB[(SQLite<br/>cafe.db)]
    W & A -. "JWT" .- AU[auth.go]
```

La API y el frontend comparten `store.go`: una sola implementación de las reglas (por ejemplo, una venta descuenta el stock en una transacción y falla si no alcanza).

## Ejecutar

```bash
cd cafe_app/app
go run .          # http://localhost:8080
```

| Variable | Por defecto | Qué hace |
|---|---|---|
| `CAFE_ADDR` | `:8080` | Dirección donde escucha |
| `CAFE_DB` | `cafe.db` | Archivo SQLite (se crea con datos de ejemplo la primera vez) |
| `CAFE_JWT_SECRET` | secreto de desarrollo | Clave con la que se firman los JWT (cámbiala al desplegar) |

Usuarios de ejemplo: `admin` / `admin123` y `cajero` / `cajero123`.

## Menús

Inventario (edición en línea; marca el stock bajo), Productos, Ventas, Clientes y Reseñas. Los formularios usan htmx: el servidor responde solo la fila nueva, y los errores aparecen en la línea roja sin recargar la página.

## API REST

Se autentica con un **JWT** (HS256, expira a las 24 horas). El navegador lo recibe en una cookie `HttpOnly` al entrar por `/login`; otros clientes lo piden con `POST /api/login` y lo envían como `Authorization: Bearer <token>`. Sin token válido (o vencido) responde `401`. No hay revocación: el token vale hasta que expira.

| Método y ruta | Qué hace |
|---|---|
| `POST /api/login` | Devuelve el JWT |
| `GET /api/products` · `POST /api/products` | Lista / crea productos |
| `GET /api/inventory` · `PUT /api/inventory/{product_id}` | Lista / fija cantidad y mínimo |
| `GET /api/customers` · `POST /api/customers` | Lista / crea clientes |
| `GET /api/sales?limit=50` · `POST /api/sales` | Lista / registra una venta (`items`: producto y cantidad) |
| `GET /api/reviews` · `POST /api/reviews` | Lista / crea reseñas |

```bash
TOKEN=$(curl -s -d '{"username":"cajero","password":"cajero123"}' localhost:8080/api/login | jq -r .token)
curl -H "Authorization: Bearer $TOKEN" -X POST -d '{"customer_id":1,"items":[{"product_id":1,"quantity":2}]}' localhost:8080/api/sales
```

## Datos de ejemplo

El **pan de queso** arranca con poco stock (3, mínimo 10) a propósito: en la fase 8 el administrador se lo pide a la panadería.
