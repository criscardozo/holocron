# Plan de mejoras — ejecutado

**Estado: T1 a T12 hechas**, publicadas en v0.6.0. Este documento se conserva
como registro de qué se cambió y por qué; las mediciones que justifican cada
tarea siguen siendo la parte útil. Lo que queda abierto son las decisiones
D1–D5 del final, que son del dueño.

Fecha del análisis: 2026-09-04, sobre `main` en `b9cbfeb` (server v0.5.2).
Este documento es el **plan de trabajo**; el análisis que lo justifica está en
cada tarea. Leer entero antes de empezar. Las tareas están en orden de
prioridad y cada una es un commit (o pocos) independiente.

## 0. Reglas que no se negocian

Están en `CLAUDE.md` del repo y del usuario; acá las que más pegan en este plan:

- **Sin CGO.** `modernc.org/sqlite`, nunca `mattn/go-sqlite3`. Se compila en la
  Mac; a la Pi va sólo el binario.
- **Todo embebido** (`//go:embed`). Sin assets sueltos.
- **Filesystem confinado con `os.Root`**, no con `filepath.Clean` + `HasPrefix`.
- **Sin JavaScript propio en la web.** Sólo HTMX vendorizado + CSS. **CSP estricta:
  prohibido `style=""` inline** (ver `docs/ui.md`; verificar con
  `curl -s localhost:8090/ | grep -o 'style="[^"]*"'` → vacío).
- **Commits en inglés (australiano), Conventional Commits**, cuerpo que explica
  *por qué*, **sin trailer de co-autoría de Claude**. Scopes en uso: `disk`,
  `naming`, `media`, `jellyfin`, `subtitles`, `torrents`, `updates`, `api`,
  `ui`, `install`, `quality`.
- **GitHub Actions: no agregar workflows, crons ni matrices sin preguntar.** El
  repo es público (los runners estándar no cobran), pero la regla de preguntar
  sigue. Cambiar pasos *dentro* de un job existente está bien.
- **iOS se compila, testea e instala en esta Mac**, nunca en CI.
- **Errores**: siempre chequeados; `fmt.Errorf("ctx: %w", err)`; loguear **o**
  propagar, no ambas. `log/slog` de baja cardinalidad.
- **Nada en el repo que identifique la cuenta**: ni emails, ni team IDs de
  Apple, ni `aud`/team de Cloudflare. El repo es público.

## 1. Cómo verificar (correr antes de cada commit)

```sh
go tool templ generate                      # tras tocar cualquier .templ
go build ./... && go vet ./...
go test -race -shuffle=on ./...
golangci-lint run
go mod tidy && git diff --exit-code -- go.mod go.sum
# iOS (simulador):
cd ios && xcodegen --spec project.yml --project . && \
  xcodebuild -project Holocron.xcodeproj -scheme Holocron \
    -destination 'platform=iOS Simulator,name=iPhone 17' test 2>&1 \
    | grep -E "Test run with|✘"
```

Instalar en el iPhone (el team y el UDID **no** van al repo; pedirlos al usuario
o leerlos de `security find-identity -v -p codesigning` y `xcrun xctrace list
devices`):

```sh
cd ios && xcodebuild -project Holocron.xcodeproj -scheme Holocron -configuration Release \
  -destination "id=$IOS_DEVICE" -derivedDataPath /tmp/hcrel \
  DEVELOPMENT_TEAM=$IOS_TEAM CODE_SIGN_STYLE=Automatic -allowProvisioningUpdates build
xcrun devicectl device install app --device "$IOS_DEVICE" \
  /tmp/hcrel/Build/Products/Release-iphoneos/Holocron.app
```

Para probar la web contra Jellyfin/qBittorrent sin la Pi, el patrón que ya se
usó: un servidor falso en Python (`http.server`) que responda los endpoints
justos, Holocron apuntado ahí con `--addr 127.0.0.1:889x --db /tmp/x.db`, y
`curl`. Hay ejemplos de fakes en `internal/quality/service_test.go` y
`internal/library/service_test.go`.

## 2. Estado medido (lo que justifica el orden)

| Área | Dato |
|---|---|
| Go | 10.609 líneas, 81 archivos. Deps directas: `templ`, `modernc.org/sqlite` (las indirectas de `x/tools`, `fsnotify`, etc. vienen del `tool templ`, no van al binario). |
| Swift | 2.461 líneas, 18 archivos. 31 tests en 6 suites, verdes. |
| Cobertura Go | `jobs` 95 %, `netaddr` 94 %, `apitoken` 91 %, `quality` 82 %, `updates` 81 %, `opensubtitles` 80 %, `db` 76 %, `scanner` 75 %, `library` 74 %, `qbittorrent` 73 %, `jellyfin` 47 %, **`subtitles` 22 %, `diskusage` 19 %, `naming` 16 %**. **Sin tests:** `httpserver`, `torrents`, `settings`, `folders`, `system`, `widgets`, `config`, `version`, `web/templates`. |
| CI | 3 jobs Linux: test+vet+build+tidy+templ-check / golangci-lint / govulncheck. **Sin gosec** (CLAUDE.md lo lista; `make check` tampoco lo corre). Release por tag, **no idempotente**. |
| Seguridad web | CSP estricta, `X-Frame-Options`, `nosniff`, `Referrer-Policy`. **Sin defensa CSRF** (ver T1). API con bearer token (SHA-256, tiempo constante). Actualización protegida por token. `os.Root` en el browse. |
| Despliegue | `holocron.merli.store` detrás de Cloudflare Access (raíz: Google; `/api`: service token). Origen del túnel `127.0.0.1:8090`. `install.sh` baja de `main` sin fijar versión. |

---

## Tareas

Formato: **Objetivo · Por qué · Archivos · Pasos · Aceptación · No hacer.**

### T1 · Rechazar POST cross-site (CSRF) — `fix(api)` · prioridad ALTA

**Objetivo.** Que la web rechace requests que cambian estado cuando vienen de
otro origen.

**Por qué.** La web no tiene sesión, así que *cualquier* request está
autorizado; un sitio malicioso abierto en un navegador de la LAN puede mandar
`POST` a Holocron. **Medido el 2026-09-04:**

```
POST /settings/folders   Origin: https://evil.example  → 303, y la carpeta quedó en la DB
POST /torrents/action    Origin: https://evil.example  → 200 (action=delete)
```

Cloudflare Access no cubre esto: el ataque ocurre desde un navegador que *sí*
tiene sesión, o desde la LAN directo al `:8090`.

**Archivos.** `internal/httpserver/middleware.go`, `server.go` (la cadena
`chain(...)`), test nuevo `internal/httpserver/middleware_test.go`.

**Pasos.**
1. Middleware `sameOrigin` que aplique a `POST`/`PUT`/`PATCH`/`DELETE` fuera de
   `/api/` (la API se autentica por bearer y la usa la app, no un navegador):
   - Si viene `Sec-Fetch-Site` y no es `same-origin` ni `none` → `403`.
   - Si no viene `Sec-Fetch-Site` pero viene `Origin`: comparar el host de
     `Origin` con `r.Host` (mismo host:puerto) → distinto → `403`.
   - Sin ninguno de los dos (curl, clientes viejos): dejar pasar. HTMX y todo
     navegador moderno mandan `Sec-Fetch-Site`.
2. Cuidado con el túnel: detrás de Cloudflare `r.Host` es
   `holocron.merli.store` y `Origin` también → same-origin. No usar `r.TLS` ni
   el esquema para comparar; comparar sólo host.
3. Loguear el rechazo como `Warn` con `origin` como atributo, sin el body.
4. Respuesta `403` con cuerpo corto; para HTMX no hace falta fragmento.

**Aceptación.** Test con `httptest`: POST con `Sec-Fetch-Site: cross-site` → 403;
con `same-origin` → pasa; con `Origin: https://evil.example` → 403; con
`Origin` igual al host → pasa; sin headers → pasa; `GET` cross-site → pasa (no
cambia estado). Y repetir a mano el `curl` de arriba: debe dar 403 y **no**
escribir en la DB.

**No hacer.** No meter tokens CSRF en cada form: sin sesión no hay dónde
anclarlos y HTMX complica el patrón. La verificación por `Sec-Fetch-Site`/`Origin`
es la defensa estándar para este caso.

### T2 · Tests de `internal/httpserver` — `test(api)` · ALTA

**Objetivo.** Cubrir la superficie HTTP, que hoy tiene **0 tests** y es donde
viven la autenticación de la API, la puerta del botón de actualizar y el CSRF.

**Por qué.** Todo lo verificado hasta ahora en handlers fue con `curl` a mano
en la sesión; nada de eso queda. Un cambio en un handler no tiene red.

**Archivos.** `internal/httpserver/*_test.go` nuevos. Hay que poder construir
un `Server` con `Deps` reales sobre una DB temporal (`db.Open` en `t.TempDir()`),
como hacen `library/service_test.go` y `quality/service_test.go`.

**Pasos.** Un helper `newTestServer(t) (*httptest.Server, deps)` y tests para:
1. `requireAPIToken`: sin header → 401; token mal → 401; sin token generado en
   el server → 503; token bueno → 200. Los mensajes JSON `{"error": …}`.
2. `handleUpdatesInstall`: sin `token` → error en el fragmento; token mal → error
   y línea de log `rejected update request`; token bueno → llama a
   `Updates.RequestInstall` (inyectar un fake o comprobar el archivo trigger).
3. `handleSaveJellyfinURL` y `handleSaveQbit`: `192.168.0.2:8096` queda guardado
   como `http://…`; `ftp://x` redirige a `/settings?notice=…` y no toca la DB.
4. `handleJellyfinTest`: sin dirección / dirección sin token (usa
   `/System/Info/Public` de un fake) / vinculado.
5. `securityHeaders`: CSP exacta, `X-Frame-Options`, `nosniff`.
6. Cada página `GET` responde 200 y **no contiene `style="`** (esto reemplaza
   el `curl | grep` manual que salvó el proyecto varias veces).
7. Quality: `POST /quality/refresh` con id que no está en el informe → badge
   «Volvé a analizar»; no admin → «Requiere admin».

**Aceptación.** `go test ./internal/httpserver/` verde; cobertura del paquete
> 60 %.

### T3 · Sacar el Plex muerto de la app iOS y poner Quick Connect — `feat(api)` · ALTA

**Objetivo.** Que la app no tenga un botón que da 404.

**Por qué.** `ios/Holocron/Views/PlexLinkView.swift` (171 líneas),
`APIClient.startPlexLink/plexLinkStatus/selectPlexServer` (3 llamadas a
`plex/link*`) y `Models.PlexServer/PlexLinkStatus` apuntan a endpoints que la
migración a Jellyfin eliminó. En Ajustes hay una sección «Plex» con «Conectar
con Plex» que hoy falla. Visto en pantalla el 2026-09-03.

**Archivos.** Los tres de arriba, `SettingsView.swift`, `MediaView.swift`
(«Plex no configurado» / «Conectar con Plex»), `HolocronTests/ContractTests.swift`
(tests `plexLinkPending/plexLinkLinked` y sus fixtures
`HolocronTests/Fixtures/plex_link_*.json`), `ios/README.md`.

**Pasos.**
1. Nuevo `JellyfinLinkStatus { state, code, user, admin }` (ver el payload en
   `docs/api.md` → «Vincular Jellyfin (Quick Connect)»). `state` es `idle |
   pending | linked | expired`.
2. `APIClient.startJellyfinLink()` → `POST jellyfin/link`;
   `jellyfinLinkStatus()` → `GET jellyfin/link`. No hay selección de servidor:
   la dirección se carga en la **web** antes (`POST /settings/jellyfin`), la
   API no la expone. La vista tiene que decirlo: «Cargá la dirección en la web
   primero» cuando el `POST` responda con ese motivo.
3. `JellyfinLinkView`: muestra el código de 6 dígitos, instrucciones «Jellyfin →
   tu perfil → Quick Connect», polling cada 2 s hasta `linked`, y al final
   `user` + si es `admin` (con la nota de que refrescar metadata lo requiere).
   Reutilizar la estructura de `PlexLinkView` y borrarla después.
4. Fixtures nuevos `jellyfin_link_pending.json` / `jellyfin_link_linked.json`
   con los campos reales; borrar los de Plex. Tests de contrato equivalentes.
5. Textos: «Plex» → «Jellyfin» en `MediaView` y `SettingsView`.

**Aceptación.** `grep -rni plex ios/` sin resultados salvo comentarios
históricos. Tests iOS verdes. Probado en simulador contra un fake que responda
`pending` y luego `linked`.

### T4 · Distinguir «token de Jellyfin vencido» de «no se pudo conectar» — `fix(jellyfin)` · MEDIA-ALTA

**Objetivo.** Que un 401 de Jellyfin (token revocado, usuario borrado, Quick
Connect re-hecho desde otro lado) se reporte como «hay que volver a vincular» y
no como falla genérica.

**Por qué.** Hoy `sync`, el `quality-scan` y el test de conexión caen en «El
trabajo falló» / «No se pudo conectar» para cualquier error. El usuario ya
perdió dos rondas esta semana por mensajes genéricos (ver historial de
`netaddr` y `Version=""`). `jellyfin.StatusError` ya expone `Status`.

**Archivos.** `internal/jellyfin/client.go` (agregar `Unauthorized() bool`),
`internal/library/service.go`, `internal/quality/service.go`,
`internal/httpserver/{media,quality,jellyfinlink}.go`, `web/templates/*`.

**Pasos.**
1. `func (e *StatusError) Unauthorized() bool { return e.Status == 401 }`.
2. En los jobs (`sync`, `scan`): envolver el error con un sentinel
   `jellyfin.ErrTokenRejected` cuando `errors.As(err, &se) && se.Unauthorized()`.
3. En los handlers de estado de job: si `errors.Is(err, ErrTokenRejected)` →
   «Jellyfin rechazó el token. Volvé a vincular desde Ajustes.» con link.
   Ojo: `jobs.Job` guarda `Err` como string hoy; o se guarda el error tipado,
   o se agrega un campo `Kind`/código al job. Elegir lo menos invasivo.
4. `handleJellyfinTest` en modo vinculado: 401 → ese mismo mensaje.
5. **No** desvincular automáticamente: sólo avisar. Borrar el token solo puede
   esconder un problema de red intermitente como si fuera revocación.

**Aceptación.** Test en `library` y `quality` con un fake que responda 401 →
el job termina en error y `errors.Is(..., ErrTokenRejected)`. Test de handler
(T2) que muestre el texto.

### T5 · `install.sh` fijado a la versión — `fix(install)` · MEDIA

**Objetivo.** Que `curl … | sudo bash` no ejecute como root lo que haya en
`main` en ese instante.

**Por qué.** `scripts/install.sh` se baja de
`raw.githubusercontent.com/<repo>/main/scripts/install.sh` y corre con `sudo`.
El binario sí se verifica por checksum; el script no. Un `main` comprometido
o simplemente roto = root en la Pi.

**Pasos.**
1. Publicar `install.sh` **como asset de cada release** (agregar al paso
   `gh release create` de `.github/workflows/release.yml` — es un cambio dentro
   del job existente, permitido) junto con su `.sha256`.
2. En el README, el comando recomendado baja el script de
   `releases/latest/download/install.sh` en vez de `main`. Y cuando el script
   se auto-actualiza o re-ejecuta (`RAW_URL`), que apunte al tag que instala.
3. Mantener compatibilidad: si se lo corre desde el repo clonado, sigue
   funcionando.

**Aceptación.** `bash -n scripts/install.sh`; el release siguiente trae el
asset; `curl -fsSL …/releases/latest/download/install.sh | sha256sum -c` cierra.

### T6 · Release idempotente — `ci` · MEDIA

**Objetivo.** Que re-pushear un tag (o re-correr el workflow) no falle con
`release already exists`.

**Archivos.** `.github/workflows/release.yml` (paso «Publish release»).

**Pasos.** `gh release view "$TAG" >/dev/null 2>&1 && gh release upload "$TAG"
<assets> --clobber || gh release create "$TAG" <assets> --title … --generate-notes`.

**Aceptación.** Re-disparar el workflow sobre un tag existente termina verde y
los assets quedan actualizados.

### T7 · gosec en `make check` y en el job de lint — `ci` · MEDIA

**Objetivo.** Que lo que dice `CLAUDE.md` («`govulncheck ./... && gosec ./...`»)
sea cierto.

**Pasos.** Agregar `gosec` a `make check` (target `sast`, instalación por
`go install github.com/securego/gosec/v2/cmd/gosec@latest`) y **un paso más**
en el job `Lint` de `ci.yml` (no un job nuevo). Corregir lo que salte o
justificar cada exclusión con `#nosec G### -- motivo` (nunca sin motivo).

**Aceptación.** `make check` verde en la Mac; CI verde.

### T8 · Cobertura en los paquetes flacos — `test` · MEDIA

**Objetivo.** `naming` (16 %), `diskusage` (19 %), `subtitles` (22 %) y los
sin tests: `torrents`, `settings`, `folders`, `system`, `widgets`.

**Pasos (por orden de riesgo).**
1. `subtitles`: la descarga escribe en disco — probar que **rechaza** un
   destino que no esté en el inventario (es la única escritura en la biblioteca
   que queda; ver `docs/arquitectura.md` §8). Fake de OpenSubtitles.
2. `naming`: casos límite del patrón `Título (Año)`: años de 4 dígitos, títulos
   con paréntesis internos, sufijos `- 1080p`, espacios, mayúsculas.
3. `diskusage`: `childDirs` salta ocultos; `DiskPath` cae a `firstExistingPath`;
   un escaneo sobre `t.TempDir()` con tamaños conocidos.
4. `torrents`: cache del cliente por `base\x00user\x00pass` (que cambiar la
   contraseña invalide el cliente) y el TTL de categorías.
5. `settings` / `folders`: CRUD sobre DB temporal; `system`: parseo de
   `/proc/stat` y `/proc/meminfo` con fixtures de texto (hoy no hay nada y en la
   Mac devuelve ceros).

**Aceptación.** Cada uno de esos paquetes > 60 %; `-race -shuffle=on` verde.

### T9 · iOS: mensaje de «no se pudo conectar» — `fix(api)` · BAJA

`APIError.notReachable` dice «¿Estás en la misma red?». Con el dominio público
ya no aplica. Cambiar a algo neutro: «No se pudo conectar con el servidor.
Revisá la dirección, y si estás fuera de casa, el service token.» Test de
mensajes existente (`APIErrorTests`) sigue pasando.

### T10 · iOS: pantalla de Calidad — `feat(api)` · MEDIA

**Objetivo.** Exponer en la app el panel que la web ya tiene.

**Por qué.** `GET /api/v1/quality`, `POST /api/v1/quality/scan` y
`POST /api/v1/quality/refresh` existen y están documentados en `docs/api.md`;
la app no los usa.

**Pasos.** `QualityReport` en `Models.swift` (`counts` es un diccionario
`[String: Int]` con claves `subs-missing | no-synopsis | generic-title | ghost |
collision`; `findings` con `category, itemId, title, detail, path, kind`).
Vista con los cinco contadores → lista por categoría → botón «Refrescar» sólo
si `admin` y la categoría es `no-synopsis` o `generic-title`. Botón «Analizar»
→ `POST scan` y polling de `GET` hasta `scanning == false`. Fixture
`quality.json` + test de contrato. Puede ir como quinta pestaña o dentro de
Medios; la nav tiene 5 tabs y una sexta ya pasa a «Más», así que **dentro de
Medios** es lo más limpio.

**Aceptación.** Contra un fake que devuelva el JSON de `docs/api.md`, la vista
muestra los contadores y la lista; `refresh` manda `item=<id>` form-encoded.

### T11 · iOS: versión alineada con el server y target de instalación — `build` · BAJA

1. `MARKETING_VERSION` en `ios/project.yml` está en `0.1.0` desde el primer día;
   el server va en 0.5.x. Ponerlo a la par de la release del server al
   publicar, o al menos actualizarlo ahora.
2. Target `ios-install` en el `Makefile` que tome `IOS_TEAM` e `IOS_DEVICE` del
   entorno (con error claro si faltan) y haga el build Release + `devicectl
   install`. **No** hardcodear el team en el repo.
3. Documentar en `ios/README.md` (hoy dice «elegilo en Xcode y dale Run»).

### T12 · Docs: `roadmap.md` — `docs` · BAJA

La sección «Ideas para más adelante» propone «Basic Auth opcional». Con
Cloudflare Access adelante ya no es la respuesta; cambiarlo por «validar el JWT
`Cf-Access-Jwt-Assertion` en el origen si algún día el túnel deja de ser la
única puerta».

---

## Decisiones que son del usuario (NO ejecutar sin que confirme)

**D1 · HTMX y el vencimiento de la sesión de Access (24 h).** Comportamiento
esperado, **a verificar**: cuando la sesión vence, un click con `hx-boost`
recibe un `302` a `*.cloudflareaccess.com`; `XMLHttpRequest` sigue el redirect,
la respuesta cross-origin queda bloqueada por CORS y HTMX dispara
`htmx:sendError` **sin mostrar nada** — la página parece muerta hasta que se
recarga a mano. Opciones: (a) aceptarlo y documentar «recargá»; (b) un `.js`
propio de 5 líneas vendorizado que escuche `htmx:sendError`/`responseError` y
haga `location.reload()` — **viola la regla «sin JavaScript propio»** de
`docs/ui.md`, así que es decisión suya; (c) `hx-headers` con
`X-Requested-With` en el `<body>` hace que Access responda `401` en lugar de
`302` (medido el 2026-09-03), pero HTMX igual no swapea un 4xx — sigue en
silencio. Primero **reproducir** (esperar el vencimiento o revocar la sesión
desde el panel de Access) y después proponer.

**D2 · Perfil de firma de 7 días.** La app está firmada con la cuenta personal
gratuita; el perfil vence cada 7 días y hay que reinstalar. Hay un team pago
disponible en el llavero pero es de una empresa. Él decide.

**D3 · `/healthz` detrás de Access.** Hoy responde 302. Si quiere un chequeo
externo de vida, esa ruta necesita su propia aplicación de Access con Bypass;
sólo devuelve `ok`. Nada lo monitorea hoy.

**D4 · CI adicional.** Como el repo es público, un job de build de iOS en
`workflow_dispatch` (macOS) y un `dependabot.yml` para módulos Go y Actions no
cuestan minutos, pero **la regla es preguntar antes de agregar workflows**.
Proponer, no agregar.

**D5 · Medición pendiente en la Pi.** La sesión «ObiWan» tiene pendiente medir
el `quality-scan` real: duración, RAM durante el decode paginado de 300, y si
molesta la reproducción. Si sale mal, `auditPageSize` en
`internal/jellyfin/items.go` es una constante.

---

## Orden sugerido y cierre

1. T1 (CSRF) → T2 (tests de httpserver, incluye el test de T1) → commit + push.
2. T4 (401 de Jellyfin) → T7 (gosec) → T6 (release idempotente) → T5 (install
   fijado).
3. T3 (Plex → Quick Connect en iOS) → T9 → T11 → reinstalar en el iPhone.
4. T8 (cobertura) → T10 (Calidad en iOS) → T12.
5. Publicar **v0.6.0**: tag anotado con notas en español, como las anteriores
   (`git tag -a v0.6.0 -m …` y `git push origin v0.6.0`); el workflow arma el
   binario arm64. Verificar checksum y versión estampada como se hizo con
   v0.5.x. Si T5 entró, verificar que `install.sh` aparezca como asset.

Al terminar cada tarea: `make check` (o el equivalente de §1), commit
convencional, push. No acumular.
