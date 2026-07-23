# Roadmap por fases

Se construye escalado: cada fase deja algo usable y desplegable en la Pi antes de
pasar a la siguiente. Los números de feature refieren a los pedidos originales.

## Fase 0 — Fundaciones

Base sobre la que se apoya todo. No entrega ninguna herramienta todavía, pero sí un
dashboard vacío que arranca en la Pi.

- Scaffold del proyecto (`go.mod`, estructura de carpetas, `Makefile`, git init).
- `config` (flags + env), `db` (SQLite + migraciones embebidas), arranque en
  `cmd/holocron/main.go` con graceful shutdown.
- `httpserver` con router stdlib, middleware (logging, gzip, headers) y assets
  embebidos.
- Layout base con templ + HTMX vendorizado + CSS.
- Framework de `widgets` y de `jobs`.
- **Widget de sistema**: CPU, RAM, temperatura, uptime y load de la Pi (lee `/proc`
  y `/sys/class/thermal`). Primer contenido real del dashboard.
- Pipeline de build y deploy (cross-compile + `scripts/deploy.sh`, unit de systemd).
- Tooling de calidad: `Makefile` con `make check` (test `-race -shuffle=on`, `vet`,
  `golangci-lint`, `govulncheck`, `gosec`) y `.golangci.yml`.

## Fase 1 — Dashboard + uso de disco (features 1 y 2)

- Grilla de widgets funcional (feature 1).
- **Widget de disco**: espacio libre/ocupado de una carpeta configurada. Click →
  página de detalle.
- **Página de detalle de disco**: escanea la carpeta y subcarpetas, muestra archivos
  y peso, con drill-down navegable.
- Portado del paquete `scanner` desde `diskusage-pi` (tamaños reales tipo `du`, stats
  vía `statfs`, drill-down seguro), integrado con `jobs` y cacheado en `scan_results`.
- ABM de carpetas vigiladas (`watched_folders`) desde la UI.

## Fase 2 — Validador de nombres (feature 3)

- `naming`: recorre las carpetas de Películas/Series y detecta las que no cumplen
  `Título (Año)`.
- **Widget** con botón de refresh chico a la derecha, que muestra el conteo de
  incumplimientos. Click → pantalla de errores.
- **Pantalla de errores**: lista de carpetas mal nombradas con lo esperado vs. lo
  encontrado. (Posible fase futura: renombrado asistido.)

## Fase 3 — Integración Plex + archivos .nfo (feature 4a)

- Portado de `plex` y `plexauth` desde `plexmatch-generator` (cliente con
  `X-Plex-Token`, login por PIN, autodescubrimiento de servidor en LAN).
- `nfo`: recorre cada directorio de medio y escribe un `.nfo` con la metadata que
  Plex ya resolvió (título, año, GUID, etc.), incluyendo un campo de si hay
  subtítulos en español.
- Inventario en `media_items`.
- Widget de estado (cuántos medios tienen/no tienen `.nfo`) + acción de generar.

## Fase 4 — Subtítulos con OpenSubtitles (feature 4b)

- `subtitles`: detección de subtítulos presentes (busca `.srt`/`.ssa`/`.sub` o lo
  declarado en el `.nfo`, embebido o aparte) y cliente de la API de
  [OpenSubtitles](https://opensubtitles.stoplight.io/docs/opensubtitles-api).
- **Widget** que lista películas/series sin subtítulos.
- Búsqueda y descarga de subtítulos desde la UI.

## Fase 5 — qBittorrent (features 5 y 6)

- `qbittorrent`: cliente de la
  [WebUI API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1))
  (login por cookie, listar/pausar/reanudar/borrar torrents).
- **Pantalla de torrents**: administración (estado, progreso, velocidades, acciones).
- **Agregar magnet-links** para descargar por BitTorrent.
- Widget de resumen (descargas activas, velocidad total) en el dashboard.

## Ideas para más adelante (fuera de alcance inicial)

- Renombrado asistido de carpetas mal nombradas (Fase 2).
- Control del servicio de Plex (reiniciar, refrescar bibliotecas).
- Notificaciones (descargas terminadas, disco casi lleno).
- Basic Auth opcional (con comparación constant-time).
- CI en GitHub Actions (test/lint/security) y releases con GoReleaser, si el repo se
  publica. No es necesario para el flujo personal de deploy por `scp`.
