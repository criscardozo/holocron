# API JSON (v1)

La API que consume la [app iOS](../ios/README.md). Vive bajo `/api/v1` y es la
**única parte del servidor con autenticación**: la interfaz web sigue abierta
dentro de la LAN (decisión consciente), pero un teléfono se va de la red, así
que la API siempre pide un token.

Se versiona para que la app pueda seguir funcionando aunque la UI HTML cambie.

## Autenticación

Todas las rutas piden un bearer token:

```
Authorization: Bearer <token>
```

El token se genera desde la web (**Ajustes → App iOS**). Detalles:

- Se crea con `crypto/rand` (256 bits) y se muestra **una sola vez**.
- Se guarda **sólo su digest SHA-256**: una copia de la base no da acceso.
- Se compara en tiempo constante (`crypto/subtle`).
- Generar uno nuevo **invalida el anterior**; también se puede revocar.

Respuestas de error de auth:

| Código | Significado |
|---|---|
| `401` | Falta el header, o el token no es válido |
| `503` | El servidor todavía no tiene ningún token generado |

Los errores siempre son `{"error": "mensaje"}`, genéricos hacia afuera; el
detalle queda en el log del servidor.

### Detrás de Cloudflare Access

Cuando el servidor se publica por un túnel con **Access** adelante, la API pide
además el service token, en dos headers:

```
CF-Access-Client-Id: <algo>.access
CF-Access-Client-Secret: <secreto>
```

Los valida Cloudflare antes de que el pedido llegue a la Pi; el bearer token se
sigue validando después. Ver la sección de seguridad en
[arquitectura.md](arquitectura.md) para las dos aplicaciones de Access que hacen
falta y por qué son dos.

Un rechazo de Access **no** se parece a un error de la API, y ni el código de
estado ni el formato del cuerpo sirven para reconocerlo. Medido contra el
despliegue real:

| Ruta | Cómo se pide | Código | `Content-Type` | Señales |
|---|---|---|---|---|
| `/api` | `Accept: application/json` | `403` | **`application/json`** | `cf-access-aud`, `cf-access-domain` |
| `/api` | `Accept: */*`, o sin `Accept` | `403` | `text/html` | `cf-access-aud`, `cf-access-domain` |
| `/api` | con headers de service token inválidos | `403` | `text/html` | `cf-access-aud`, `cf-access-domain` |
| raíz | navegación normal, o `Accept: application/json` | `302` | `text/html` | `location` a `*.cloudflareaccess.com`, `WWW-Authenticate` |
| raíz | con `X-Requested-With: XMLHttpRequest` | `401` | `text/html` | `WWW-Authenticate: Cloudflare-Access` |

Dos asimetrías medidas que conviene no razonar por analogía:

- **La negociación de contenido existe sólo en la app de Service Auth.** En la
  raíz el `Accept` no cambia nada: siempre redirige con HTML.
- **Lo que dispara el `401` en lugar del `302` es `X-Requested-With`**, no el
  `Accept`. Y `WWW-Authenticate` está en **todas** las respuestas de la raíz,
  también en el `302`, pero en **ninguna** de `/api`.

Cuidado con la tentación de asumir que **Access nunca devuelve JSON**: es falso,
y el esquema no se parece en nada al de Holocron:

```json
{"message": "Forbidden. You don't have permission to view this…",
 "status_code": 403, "aud": "ddb3…", "ray_id": "a353…",
 "ip_address": "…", "is_warp": false, "is_gateway": false, "mtls_status": "NONE"}
```

Usa `message`, no el `error` de Holocron, así que un cliente que sólo lea
`error` muestra un mensaje vacío. Y **trae `ip_address` con la IP pública de
quien llamó**: no loguear el cuerpo crudo de un error ni mostrarlo en pantalla.
El `aud` del cuerpo repite el header, así que en el caso JSON la señal viene
duplicada.

Lo único presente en **todos** los rechazos son los headers propios de Access
(`cf-access-aud`, `cf-access-domain`), que no dependen del código, del cuerpo ni
del redirect. Es la señal en la que conviene apoyarse; el resto quedan como
respaldo por si alguna versión deja de anunciarse.

Con las dos capas puestas y **sin** el bearer token de Holocron, la respuesta es
`401 application/json {"error":"missing bearer token"}`: Access dejó pasar y
Holocron pidió lo suyo. Que se distingan es justamente el punto.

### Subtítulos

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/subtitles` | Medios sin subtítulo en español |
| `GET` | `/api/v1/subtitles/search?title=…&year=…` | Busca en OpenSubtitles |
| `POST` | `/api/v1/subtitles/download` | Descarga uno al directorio del medio |

`missing` es el total real, que puede ser mayor que `items` (cortado en 500).

El `POST` recibe `{"fileId": 123, "path": "/mnt/media/…"}`. **`path` se valida
contra el inventario**: sólo se puede escribir en carpetas que Holocron
registró desde Jellyfin; cualquier otra cosa es `400`.

### Torrents

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/torrents` | Lista + totales de actividad |
| `POST` | `/api/v1/torrents` | Agrega un magnet: `{"magnet": "magnet:?…"}` |
| `POST` | `/api/v1/torrents/{hash}/{pause\|resume\|delete}` | Acción sobre uno |

```json
{
  "configured": true, "total": 4, "active": 1,
  "dlSpeed": 4300000, "upSpeed": 655360,
  "categories": ["Docs", "Peliculas", "Series"],
  "torrents": [{
    "hash": "a1b2…", "name": "Cosmos.S01E03.1080p.WEB", "state": "downloading",
    "category": "Series", "progress": 0.63, "sizeBytes": 2576980377,
    "dlSpeed": 4300000, "upSpeed": 215040,
    "seeds": 38, "leechs": 5, "paused": false
  }]
}
```

`categories` son las definidas en qBittorrent, ordenadas alfabéticamente, para
ofrecerlas al agregar un magnet: `POST /api/v1/torrents` acepta
`{"magnet": "…", "category": "Series"}`. Una `category` vacía (o ausente) deja
el torrent sin categoría, en la carpeta por defecto. Si no se pudieron leer las
categorías, el campo viene vacío y el resto de la respuesta sigue sirviendo.

`state` es el estado crudo de qBittorrent; `paused` ya viene resuelto. El
cliente deriva la etiqueta visible del par (`state`, `paused`) — ver
`Torrent.Status` en la app.

`delete` **no borra los archivos**, sólo el torrent.

Códigos: `400` magnet inválido, `412` qBittorrent sin configurar, `502` no se
pudo hablar con qBittorrent.

## Convenciones

- Todos los tamaños en **bytes**, las velocidades en **bytes por segundo** y
  `progress` de 0 a 1. El formateo es cosa del cliente.
- Los timestamps son UTC con el formato de SQLite (`2006-01-02 15:04:05`); el
  cliente los convierte a hora local.
- Los cuerpos de request están limitados a 1 MiB.
- Los trabajos pesados son asincrónicos: `202` y después polling.

## Estabilidad

Los tests de contrato de la app (`ios/HolocronTests/ContractTests.swift`)
decodifican **respuestas capturadas de un servidor real**, así que un cambio de
nombre o de tipo en un campo se detecta ahí en vez de aparecer como un bug en el
teléfono. La app **no se compila en CI** (el runner de macOS costaba varios
minutos por push), así que hay que correrlos a mano con `make ios-test` después
de tocar la forma de una respuesta, y regenerar los fixtures (ver
`ios/HolocronTests/Fixtures/README.md`).
