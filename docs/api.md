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

## Endpoints

### Sistema

`GET /api/v1/system`

Métricas de la Pi. Cada valor es `null` cuando no se puede leer (fuera de Linux
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

La dirección del servidor se carga **antes** del código (`POST /settings/jellyfin`
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
