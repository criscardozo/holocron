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

### Detrás de Caddy

Holocron escucha en `127.0.0.1:8090` y se entra por Caddy, con HTTPS, desde
casa o por Tailscale; nada queda expuesto a internet. El servidor confía en
`X-Forwarded-For` **sólo** si el pedido llega por loopback, y toma la última
entrada, que es la que agregó Caddy. Con eso decide si un pedido viene de casa
(ver `remote` en Gestión); el rango de Tailscale (`100.64.0.0/10`) cuenta como
afuera.

## Endpoints

### Sistema

`GET /api/v1/system`

Métricas del equipo. Cada valor es `null` cuando no se puede leer (fuera de Linux
no existe `/proc`), así que el cliente debe tolerar nulos.

```json
{
  "cpuPercent": 34.2, "memUsedBytes": 3221225472, "memTotalBytes": 8589934592,
  "memPercent": 37.5, "tempCelsius": 52.1, "uptimeSeconds": 1051200,
  "load1": 0.82, "hostname": "raspberrypi"
}
```

### Disco

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/disk` | Carpetas vigiladas con su uso |
| `GET` | `/api/v1/disk/{id}` | Detalle: uso, estado del escaneo y carpetas más grandes |
| `GET` | `/api/v1/disk/{id}/browse?path=…` | Un nivel del drill-down |
| `POST` | `/api/v1/disk/{id}/scan` | Dispara el escaneo (202; es asincrónico) |

`GET /api/v1/disk`:

```json
{"folders": [{
  "id": 1, "label": "Películas", "path": "/mnt/media/peliculas",
  "totalBytes": 2000000000000, "usedBytes": 1500000000000,
  "freeBytes": 500000000000, "usedPercent": 75, "available": true
}]}
```

`available: false` significa que no se pudo leer el filesystem (disco
desconectado); en ese caso los tamaños vienen en cero.

`GET /api/v1/disk/{id}` agrega `scanning` (bool), `scannedAt` (timestamp UTC,
sólo si hay un escaneo cacheado) y `top` (las carpetas más grandes). El escaneo
es un trabajo en background: se dispara con `POST …/scan` y se consulta
`scanning` hasta que vuelve en `false`.

`browse` sin `path` lista la raíz de la carpeta. **El servidor confina la ruta
con `os.Root`**: cualquier intento de salir del root configurado da `400`.

### Nombres

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/naming` | Carpetas que no cumplen «Título (Año)» |
| `POST` | `/api/v1/naming/scan` | Re-escanea (es barato, responde sincrónico) |

```json
{"count": 1, "issues": [{
  "path": "/mnt/media/peliculas/Interstellar 2014", "type": "movies",
  "found": "Interstellar 2014", "expected": "Interstellar (2014)"
}]}
```

`type` es `movies` o `tv`.

### Vincular Jellyfin (Quick Connect)

Obtiene el token de Jellyfin sin que el usuario busque una API key.

| Método | Ruta | Qué hace |
|---|---|---|
| `POST` | `/api/v1/jellyfin/link` | Pide un código y arranca el flujo |
| `GET` | `/api/v1/jellyfin/link` | Estado actual (se consulta en loop) |

```json
{"state": "pending", "code": "640045", "user": "", "admin": false}
```

`state` es `idle`, `pending`, `linked` o `expired`. El flujo es:

1. `POST` → devuelve un `code` de 6 dígitos. Se le muestra al usuario.
2. El usuario lo aprueba en Jellyfin, en su perfil → **Quick Connect**.
3. `GET` cada ~2 s hasta que `state` pase a `linked`. **En ese momento el
   servidor ya guardó el token.**

Cuando queda vinculado, `user` trae quién autorizó y `admin` si esa cuenta es
administradora — pedirle a Jellyfin que escriba metadata requiere serlo, así que
conviene avisarlo antes que fallar después.

La dirección se **normaliza** al guardarla: `192.168.0.2:8096` se guarda como
`http://192.168.0.2:8096`. Sin esquema no es una URL —`net/url` lee los dos
puntos como separador de esquema— y el request no se llega a armar, así que
todas las llamadas fallaban con un «no se pudo conectar» genérico.

La dirección se carga **antes** del código (`POST /settings/jellyfin`
en la web): a diferencia de Plex no hay un servicio en la nube por el que
descubrir servidores. Si Quick Connect está desactivado en Jellyfin, el `POST`
responde con ese motivo — es un toggle del panel del servidor.

### Medios

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/media` | Inventario de Jellyfin + contadores |
| `POST` | `/api/v1/media/sync` | Sincroniza desde Jellyfin (202) |

```json
{
  "configured": true, "total": 344, "movies": 298, "withoutSubsEs": 12,
  "syncing": false, "truncated": false,
  "items": [{
    "path": "/mnt/media/peliculas/Dune Parte Dos (2024)",
    "title": "Dune: Parte Dos", "year": 2024, "type": "movie",
    "hasSubsEs": false
  }]
}
```

Con `configured: false` (Jellyfin sin vincular) el resto de los campos se omiten
y `items` viene vacío. La lista se corta en 500 ítems; `truncated` lo indica.

Los dos `POST` devuelven `202` tanto si arrancaron el trabajo como si ya había
uno corriendo, y `412` si Jellyfin no está vinculado.

### Gestión de la máquina

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/manage` | Acciones disponibles y qué se interrumpiría |
| `POST` | `/api/v1/manage/action` | Pide una acción (202). `action=<clave>`; `ack=1` es obligatorio para `poweroff` si el pedido no viene de casa; si no, responde 428 |

```json
{
  "available": true,
  "actions": [
    {"key": "restart-jellyfin", "label": "Reiniciar Jellyfin",
     "detail": "…", "needsToken": false, "interrupts": false},
    {"key": "poweroff", "label": "Apagar el equipo",
     "detail": "…", "needsToken": true, "interrupts": true}
  ],
  "warnings": ["Se está reproduciendo Chernobyl · S01E01 (cris)"],
  "checked": true,
  "remote": false,
  "machine": "Ginebra",
  "lastAction": {"action": "reboot", "ok": true, "reason": "", "at": "2026-10-05T21:14:03Z"}
}
```

`warnings` es lo que se interrumpiría ahora mismo. **`checked` distingue «no hay
nada en curso» de «no se pudo preguntar»**: los dos dan una lista vacía y sólo
uno significa que es seguro apagar.

`interrupts` marca las que se llevan la máquina entera. `needsToken` marca la
única irreversible: la API ya está autenticada por bearer, así que el campo no
cambia lo que valida el server — **le dice al cliente que esa acción merece
ceremonia propia**. La app la pide manteniendo apretado y, cuando `remote` es
`true`, además con un interruptor que dice que el equipo no se puede encender a
distancia.

`remote` dice si **este** pedido llegó desde afuera de casa, según el servidor
(ver «Detrás de Caddy»). `machine` es el nombre del equipo para los títulos.
`lastAction` es el resultado de la última acción que corrió el ayudante con
privilegios, y falta si nunca corrió ninguna.

`action=<key>` va form-encoded. El `202` llega **antes** de que la máquina
actúe: la unit espera un par de segundos justamente para que la respuesta
salga, porque un cliente que ve caer la conexión sin respuesta no puede
distinguir un rechazo de un éxito. Después de ese acuse, **perder contacto es la
confirmación del éxito**, no un error.

Responde `412` si el ayudante con privilegios no está instalado.

### Calidad de biblioteca

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/quality` | El último informe cacheado |
| `POST` | `/api/v1/quality/scan` | Corre un análisis nuevo (202) |
| `POST` | `/api/v1/quality/refresh` | Le pide a Jellyfin releer un ítem (202) |

```json
{
  "configured": true, "hasReport": true, "scanning": false, "admin": true,
  "generatedAt": "2026-08-26T00:46:10Z", "scanned": 2336, "total": 1804,
  "counts": {
    "subs-missing": 1225, "no-synopsis": 500, "generic-title": 46,
    "ghost": 87, "collision": 4
  },
  "findings": [{
    "category": "collision", "itemId": "a1b2c3",
    "title": "Chernobyl · S01E01 · 1:23:45",
    "detail": "S01E01 aparece en 2 archivos",
    "path": "/mnt/media/series/Chernobyl/S01/ep0.mkv",
    "kind": "episodio"
  }]
}
```

El `GET` **nunca** analiza: leer la biblioteca entera de Jellyfin (episodios
incluidos) es un `POST` explícito. `counts` trae los totales reales; `findings`
se corta en 200 por categoría, así que un contador puede ser mayor que la
cantidad de hallazgos devueltos.

`refresh` toma `item=<itemId>` form-encoded y sólo acepta ids que estén en el
informe vigente: el id llega del cliente y termina en una escritura sobre el
servidor de medios. Responde `404` si no lo reconoce y `403` si la cuenta de
Jellyfin vinculada no es administradora (releer metadata lo requiere).

### Pantallas en vivo

| Método | Ruta | Qué hace |
|---|---|---|
| `GET` | `/api/v1/hardware` | CPU por núcleo, memoria, red, discos y batería |
| `GET` | `/api/v1/activity` | Reproducciones, descargas, pedidos de Seerr, calendario y avisos |
| `GET` | `/api/v1/services` | Unidades de systemd, tareas programadas, SMART y la deriva de la instalación (`drift`, `null` si el servidor no la chequea) |

Devuelven **el mismo view model que renderiza la web**, ya formateado en el
servidor (`"7 %"`, `"146.5 KiB/s"`, `"hace 3 h"`), así el teléfono y el
navegador dicen lo mismo y el cliente no decide qué significa cada estado. Los
largos de barra vienen como texto sobre un lienzo de 0 a 100 (`"width": "20.0"`)
y los sparklines como los puntos del polyline sobre un lienzo de 100×24.

Las listas vacías pueden llegar como `null`: el cliente tiene que tolerarlo.

La web recibe esto mismo por SSE (`/events/…`); la API se consulta. El servidor
muestrea sólo mientras alguien pregunta, así que un cliente que deja de preguntar
cuando la pantalla no se ve no le cuesta nada al equipo.

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
