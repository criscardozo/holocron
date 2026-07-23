# Arquitectura y decisiones técnicas

Este documento fija las decisiones estructurales de Holocron. Las features
concretas se describen en [features.md](features.md); el orden de construcción en
[roadmap.md](roadmap.md).

## 1. Principios

1. **Un binario, cero dependencias en destino.** La Raspberry Pi solo recibe un
   ejecutable. No se instala Go, ni Node, ni compiladores, ni librerías del sistema.
2. **Se compila en la MacBook.** Todo el toolchain (Go, `templ generate`) vive en la
   máquina de desarrollo. Ver [Compilación y despliegue](#compilación-y-despliegue).
3. **Sin CGO.** Esto es lo que hace posible el punto 1 y 2: cross-compilar a ARM sin
   un toolchain de C. Condiciona la elección de librerías (ver SQLite).
4. **Bajo demanda.** Los escaneos y trabajos pesados los dispara el usuario; no hay
   procesos corriendo en loop consumiendo la Pi. Los resultados se cachean.
5. **Modular.** Cada feature es un paquete `internal/` independiente + uno o más
   widgets en el dashboard. Sumar una feature no toca a las demás.

## 2. Stack

| Capa | Elección | Motivo |
|---|---|---|
| Lenguaje | Go 1.23+ | Liviano, binario estático, cross-compila a ARM. |
| HTTP | `net/http` (stdlib) | Router con métodos de Go 1.22 (`GET /ruta`). Sin framework. |
| Vistas | [templ](https://templ.guide) | Componentes tipados que compilan a Go. Se genera en la Mac. |
| Interactividad | [HTMX](https://htmx.org) | Updates parciales (refresh de widgets) sin escribir JS ni SPA. Se vendoriza embebido (~14 KB). |
| Estilos | CSS moderno propio (custom properties) | Sin build step. Igual criterio que `diskusage-pi`. |
| Persistencia | SQLite vía `modernc.org/sqlite` | Driver **Go puro, sin CGO**. Un archivo, cero servidor de DB. |
| Logging | `log/slog` (stdlib) | Estructurado, sin dependencias. |

**Dependencias externas totales: dos** (`github.com/a-h/templ` y
`modernc.org/sqlite`). Todo lo demás es stdlib. HTMX no es una dependencia de Go: es
un `.js` vendorizado y embebido.

> **Nota sobre SQLite:** el driver popular `mattn/go-sqlite3` **queda descartado**
> porque requiere CGO y un compilador C en el destino, lo que rompería la
> cross-compilación. `modernc.org/sqlite` es una traducción del motor a Go puro.

## 3. Estructura del proyecto

```
holocron/
  cmd/holocron/
    main.go                 # arranque, flags/env, señales, graceful shutdown
  internal/
    config/                 # config del server (flags + env) y helpers
    db/                     # conexión SQLite, migraciones embebidas, queries
    jobs/                   # runner de trabajos en background con estado y progreso
    system/                 # stats de la Pi: CPU, RAM, temperatura, uptime, load
    scanner/                # uso de disco (portado de diskusage-pi)
    naming/                 # validador de convención "Título (Año)"
    plex/                   # cliente de Plex Media Server (portado de plexmatch-generator)
    plexauth/               # login por PIN + autodescubrimiento (portado)
    nfo/                    # generación de archivos .nfo desde metadata de Plex
    subtitles/              # cliente de OpenSubtitles + detección de subs presentes
    qbittorrent/            # cliente de la WebUI API de qBittorrent
    widgets/                # registro de widgets del dashboard
    httpserver/             # rutas, middleware, wiring
  web/
    templates/              # archivos .templ (layout, dashboard, páginas por feature)
    static/                 # htmx.min.js, styles.css, favicon — embebidos con //go:embed
  docs/                     # esta documentación (en español)
  packaging/                # unit de systemd, config de nfpm (.deb) opcional
  scripts/                  # build.sh (cross-compile), deploy.sh (scp a la Pi)
  Makefile
  go.mod
  CLAUDE.md
```

Los paquetes `scanner`, `plex`, `plexauth` se **portan** desde los proyectos
hermanos, no se reescriben de cero. Se adaptan a la interfaz de `jobs` y a las
tablas de `db`.

## 4. Modelo de datos (SQLite)

La base vive en un único archivo (por defecto `~/.local/share/holocron/holocron.db`,
configurable). Migraciones SQL embebidas con `//go:embed`, aplicadas al arrancar.

Tablas previstas (crecen por fase):

- `settings` — pares clave/valor para config editable desde la UI (rutas de
  bibliotecas, API keys, URL/credenciales de Plex y qBittorrent).
- `watched_folders` — carpetas que el usuario elige vigilar (para el widget de disco
  y el validador de nombres): `id, label, path, purpose`.
- `scan_results` — cache del último escaneo de disco por carpeta (JSON + timestamp).
- `naming_issues` — resultados del validador de nombres: `path, type, expected,
  found, resolved`.
- `media_items` — inventario de medios detectados (para .nfo y subtítulos): `path,
  type, title, year, plex_guid, has_subs_es, nfo_written_at`.
- `jobs` — trabajos en background: `id, kind, status, progress, error, started_at,
  finished_at, result`.

> Config sensible (API keys, tokens) se guarda en SQLite con permisos owner-only
> sobre el archivo de la DB. No se versiona.

## 5. Trabajos en background (`jobs`)

Varias features son lentas (escanear un disco grande, generar .nfo de toda la
biblioteca, buscar subtítulos contra una API). No pueden bloquear un request HTTP.

`jobs` provee:

- Lanzar un trabajo por su `kind` (p. ej. `disk-scan`, `nfo-generate`).
- Estado con máquina simple: `idle → running → done | error`, con `progress`
  (0–100) y contadores, inspirado en el cache de estados de `diskusage-pi-claude`.
- La UI hace polling con HTMX (`hx-trigger="every 2s"`) a un endpoint de estado
  mientras el trabajo corre, y muestra el resultado al terminar.
- Concurrencia acotada (un worker por `kind`, para no saturar la Pi).

## 6. Dashboard y widgets (`widgets`)

El dashboard es una grilla de widgets. Cada widget implementa una interfaz común:

- `Render()` → fragmento HTML (templ) con el resumen y, si aplica, un botón de
  refresh chico a la derecha.
- Un endpoint de refresh (`GET /widgets/{id}`) que devuelve solo el fragmento
  actualizado (HTMX lo swapea in-place).
- Un link opcional a una **página de detalle** (la pantalla completa de la feature).

Esto cumple los pedidos: paneles en grilla (feature 1), widget de disco clickeable
(2), widget de validación con refresh que linkea a la pantalla de errores (3), etc.

Patrones de HTMX a usar (fragmentos mínimos, sin JS propio):

- **`hx-boost`** en la navegación dashboard ↔ páginas de detalle: navegación tipo
  SPA sin recargar todo, con degradación elegante si no hay JS.
- **`hx-indicator`** para el estado "cargando" mientras corre un trabajo o refresca
  un widget (spinner chico en el widget).
- **Polling** con `hx-trigger="every 2s"` solo mientras un job está `running`; el
  fragmento deja de pedir solo cuando el job termina (el server devuelve el resumen
  final sin el atributo de polling).
- **`hx-confirm`** en toda acción destructiva (borrar torrent, quitar carpeta
  vigilada, sobrescribir .nfo).
- Cada endpoint de fragmento devuelve **solo el HTML necesario**, no la página.

## 7. Configuración del server

Dos niveles, igual criterio que `diskusage-pi`:

- **Arranque** (flags + variables de entorno; los flags pisan al entorno):
  `--addr` (default `:8080`), `--db`, `--log-level`. Sin TLS por ahora (LAN de
  confianza, sin auth — decisión tomada).
- **Aplicación** (editable desde la UI, persistida en `settings`): rutas de medios,
  credenciales de Plex/qBittorrent, API key de OpenSubtitles.

## 8. Seguridad

- Sin autenticación en esta etapa (red interna de confianza). El diseño deja lugar
  para sumar Basic Auth como middleware sin tocar el resto; cuando se agregue, la
  comparación de credenciales será **constant-time** (`crypto/subtle`).
- **Path traversal**: todo acceso al filesystem (escaneo, drill-down, escritura de
  .nfo) se confina a las carpetas configuradas usando **`os.Root`** (Go 1.24+), que
  ata las operaciones a un directorio raíz y bloquea el escape vía `..` o symlinks a
  nivel del kernel. Se descarta el patrón `filepath.Clean` + `strings.HasPrefix`
  (frágil, desaconsejado por las guías de seguridad de Go). Nota: el `scanner`
  portado de `diskusage-pi` usa el enfoque viejo; al integrarlo se migra a `os.Root`.
- **Errores hacia el usuario**: mensajes genéricos en la UI; el detalle técnico va al
  log del servidor. Nunca exponer rutas internas, stack traces ni errores crudos de
  la DB o de APIs externas al navegador.
- **Secretos** (tokens de Plex, API key de OpenSubtitles, credenciales de
  qBittorrent): en SQLite con permisos owner-only sobre el archivo; nunca en logs ni
  en el binario. Client IDs/tokens que haya que generar usan `crypto/rand`.
- **Bind**: `--addr` por defecto escucha en todas las interfaces (`:8080`) porque el
  dashboard se accede desde otras máquinas de la LAN. Es una decisión consciente
  (LAN de confianza); se puede acotar a una IP específica vía el flag.
- CSP estricta en headers, HTMX y CSS servidos desde el mismo origen (embebidos).

## 9. Errores y logging

Convenciones (stdlib, sin dependencias extra):

- Errores **siempre chequeados**, nunca descartados con `_`.
- Wrapping con contexto: `fmt.Errorf("scan %s: %w", path, err)`; inspección con
  `errors.Is` (centinelas) y `errors.As`/`errors.AsType` (tipos).
- **Single handling rule**: un error se loguea **o** se propaga, nunca ambas cosas
  (evita logs duplicados).
- Logging estructurado con **`log/slog`**. Mensajes de log de baja cardinalidad
  (plantilla estable); los datos variables (rutas, IDs, conteos) van como atributos.
- Middleware que loguea cada request HTTP (método, path, status, duración).

> Se mantiene solo stdlib. Librerías como `samber/oops` o `tint` quedan como opción
> futura si algún día se quiere stack traces o salida coloreada, pero no se suman
> ahora para respetar el principio de dependencias mínimas.

## 10. Calidad y verificación

Todo corre en la Mac (o en CI opcional). El objetivo es que un binario que llega a
la Pi ya pasó por estos filtros:

- `go test -race -shuffle=on ./...` — tests con detección de data races y orden
  aleatorio.
- `go vet ./...` y **golangci-lint** (config en `.golangci.yml`).
- `go mod tidy` + `git diff --exit-code` — falla si `go.mod`/`go.sum` quedaron sucios.
- `govulncheck ./...` — vulnerabilidades conocidas en el código realmente alcanzado.
- `gosec ./...` — análisis estático de seguridad.

Estos comandos se agrupan en el `Makefile` (`make check`). Un workflow de GitHub
Actions que los corra en cada push es **opcional** y se puede sumar más adelante
(ver `docs/roadmap.md`); no es necesario para el flujo personal de deploy por `scp`.

## Compilación y despliegue

**Se compila en la MacBook. La Raspberry Pi solo recibe el binario.**

1. Generar las vistas (solo en la Mac, requiere el CLI de templ):
   ```
   templ generate
   ```
2. Cross-compilar para la Pi. Target confirmado: **Raspberry Pi 4/5 con SO de 64
   bits → `arm64`**:
   ```
   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o dist/holocron ./cmd/holocron
   ```
3. Copiar el binario a la Pi:
   ```
   scp dist/holocron pi@raspberry:/usr/local/bin/holocron
   ```
4. En la Pi corre como servicio de **systemd** (unit provista en `packaging/`).
   Alternativa: empaquetar un `.deb` con nfpm para instalar/actualizar prolijo, como
   ya hace `diskusage-pi`.

Todo esto se automatiza en el `Makefile` y `scripts/` (`make build`, `make deploy`).

> **Opción a futuro — GoReleaser**: si se quiere versionar releases prolijos
> (binario `arm64` + checksums + changelog), GoReleaser en modo `--snapshot` local
> hace el cross-compile en la Mac sin depender de GitHub. Es opcional; el `Makefile`
> alcanza para el flujo personal.

> **Target confirmado:** Raspberry Pi 4/5 con SO de 64 bits → `GOARCH=arm64`.
