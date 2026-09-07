# Especificación de features

Detalle de cada feature: qué hace, qué endpoints/pantallas expone, qué guarda y qué
reutiliza. El orden de construcción está en [roadmap.md](roadmap.md).

---

## Feature 1 — Dashboard con grilla de paneles

Pantalla principal: una grilla responsive de widgets. Cada widget muestra un resumen
y, según el caso, un botón de refresh chico arriba a la derecha y/o un link a su
página de detalle.

- **UI**: grilla CSS. Cada widget es un componente templ con un fragmento
  refrescable vía HTMX.
- **Endpoint**: `GET /` (dashboard), `GET /widgets/{id}` (fragmento de un widget).
- **Base para**: todas las demás features enganchan acá su widget.

---

## Feature 2 — Uso de disco de una carpeta

**Widget**: espacio libre/ocupado de una carpeta que el usuario configura. Click →
página de detalle.

**Página de detalle**: escanea la carpeta y subcarpetas y muestra archivos y peso,
con drill-down navegable (entrar carpeta por carpeta viendo el tamaño de cada hijo).
Equivale a lo que hace `diskusage-pi`.

- **Reutiliza**: paquete `scanner` de `diskusage-pi`. Ya calcula tamaños asignados
  reales (`st_blocks × 512`, como `du`), stats del filesystem con `syscall.Statfs`,
  top-N de carpetas, y drill-down (`Browse`) con validación anti path-traversal.
- **Datos**: `watched_folders` (carpetas elegidas), `scan_results` (cache del último
  escaneo). Escaneo vía `jobs` (`kind: disk-scan`) con progreso.
- **Endpoints**: `GET /disk` (detalle), `GET /disk/browse?path=…` (drill-down),
  `POST /disk/scan` (dispara escaneo), config de carpetas en `GET/POST /settings`.

---

## Feature 3 — Validador de convención "Título (Año)"

**Widget**: botón de refresh chico a la derecha; muestra cuántas carpetas de
Películas/Series no cumplen la norma `Título (Año)`. Click → pantalla de errores.

**Pantalla de errores**: lista de carpetas mal nombradas, con lo esperado vs. lo
encontrado, agrupadas por biblioteca.

- **Lógica** (`naming`): recorre el primer nivel de las carpetas de biblioteca
  configuradas y valida cada nombre contra un patrón `^.+ \(\d{4}\)$` (con matices:
  detectar año presente pero mal formateado, sufijos como `{edition-...}`, etc.).
- **Datos**: `naming_issues` (`path, type, expected, found, resolved`).
- **Endpoints**: `GET /naming` (pantalla), `POST /naming/scan` (refresh), `GET
  /widgets/naming` (fragmento del widget).
- **Futuro**: renombrado asistido (sugerir el nombre correcto y aplicarlo).

---

## Feature 4a — Inventario de medios desde Jellyfin

Holocron inventaría la biblioteca desde Jellyfin: título, año, identificadores
externos y si hay subtítulos en español. **Los subtítulos los reporta Jellyfin**,
que ya conoce cada pista de cada archivo (incluidas las embebidas), así que no se
recorre el disco para averiguarlo.

> Holocron **no escribe `.nfo`**. Lo hizo hasta la v0.4.1: Jellyfin escribe los
> mismos archivos cuando la biblioteca tiene activado «guardar metadata», y dos
> escritores sobre la misma ruta significa que gana el último. El dueño es
> Jellyfin; Holocron sólo lee.

- **Cliente** (`jellyfin`): Quick Connect para el token, e inventario con
  `GET /Items` **sin** `userId` — al filtrar por usuario, Jellyfin esconde las
  películas que pertenecen a una colección (46 de 344 en la biblioteca real) y a
  la vez devuelve las colecciones como si fueran títulos. Se descartan los ítems
  sin `Path`: son los que existen en el proveedor de metadata pero no en disco.
- **Datos**: `media_items` (`path, type, title, year, server_item_id,
  provider_ids, has_subs_es`). El sync corre por `jobs` (`kind: media-sync`).
- **Endpoints**: `GET /media` (inventario), `POST /media/sync` (traer de
  Jellyfin), y en Ajustes `POST /settings/jellyfin` +
  `/settings/jellyfin/link` para la dirección y el Quick Connect.
- **A cuidar**: `ProviderIds` se modela como mapa, no como struct, porque trae
  claves con espacios (`"official website"`). Un título puede tener varios
  archivos (misma película en 1080p y 4K) y los subtítulos cuelgan de uno solo,
  así que hay que unir las pistas de todos.

---

## Feature 4c — Panel de calidad de biblioteca

**Pantalla** con cinco contadores, cada uno con su lista. Responde «qué le falta
a la biblioteca», que es distinto de «cuánto hay»:

| Categoría | Qué es |
|---|---|
| Sin subtítulos ES | Ni pista en español ni audio en español |
| Sin sinopsis | Jellyfin no tiene descripción del ítem |
| Título genérico | `Episode 4`, o el nombre del archivo: no lo identificó |
| Fantasmas | Jellyfin lo lista y el archivo ya no está |
| Numeración repetida | Dos episodios con el mismo `SxxEyy`; uno queda tapado |

- **Datos**: un único documento JSON en `quality_reports` (`CHECK (id = 1)`),
  reemplazado por cada análisis. Cubre **episodios**, que no están en
  `media_items` — meter dos mil filas de episodio ahí cambiaría lo que significa
  el inventario.
- **Análisis** (`quality`): `jobs` con `kind: quality-scan`. Pide a Jellyfin
  `Movie,Series,Episode` **paginado** (los episodios traen sus pistas, así que
  pedir dos mil de una es un decode de varios MB en la Pi) y clasifica en
  memoria. `Analyse` es una función pura sobre la respuesta, así que las reglas
  se testean contra respuestas capturadas y no contra un servidor prendido.
- **Reglas que importan**: una serie es un directorio y no tiene pistas propias,
  así que **no** se le pregunta por subtítulos (sería un falso hallazgo por cada
  serie). Un fantasma se reporta una sola vez, como fantasma. Dos episodios «sin
  número» no colisionan entre sí.
- **Acción**: donde el problema es metadata (sin sinopsis, título genérico) hay
  un botón que le pide a Jellyfin un `FullRefresh` del ítem. Requiere cuenta
  administradora —se avisa antes, no después del 403— y el id se valida contra
  el informe vigente antes de mandar nada.
- **Tope**: se muestran 200 hallazgos por categoría; los contadores son los
  totales reales y la UI dice cuándo la lista está cortada.

---

## Feature 4b — Búsqueda de subtítulos (OpenSubtitles)

**Widget**: lista de películas/series **sin** subtítulos. Desde ahí se pueden buscar
y descargar.

- **Detección de subtítulos presentes** (`subtitles`): para cada medio, se considera
  que tiene subtítulos si existe un archivo `.srt`/`.ssa`/`.sub` junto al video, o si
  Jellyfin declara una pista de subtítulos (embebida o aparte).
- **Búsqueda/descarga**: cliente de la
  [API de OpenSubtitles](https://opensubtitles.stoplight.io/docs/opensubtitles-api/e3750fd63a100-getting-started).
  Requiere API key (se guarda en `settings`). Búsqueda por título/año o por hash del
  archivo; descarga del `.srt` al directorio del medio.
- **Endpoints**: `GET /subtitles` (faltantes), `POST /subtitles/search`,
  `POST /subtitles/download`.

---

## Feature 5 — Administración de torrents (qBittorrent)

Administra qBittorrent (qbittorrent-nox) vía su
[WebUI API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)).

- **Cliente** (`qbittorrent`): login (`/api/v2/auth/login`, cookie de sesión), listar
  (`/api/v2/torrents/info`), pausar/reanudar/borrar, ver estado global.
- **Pantalla**: tabla de torrents con estado, progreso, velocidades, seeds/peers y
  acciones. Widget de resumen (activos + velocidad total) en el dashboard.
- **Datos**: URL y credenciales de qBittorrent en `settings`.
- **Endpoints**: `GET /torrents`, `POST /torrents/{hash}/{action}`.

---

## Feature 6 — Agregar magnet-links

Agregar descargas por magnet-link desde la UI.

- Usa `/api/v2/torrents/add` de la WebUI API (acepta `urls` con el magnet).
- **UI**: un input para pegar el magnet (validación básica del esquema `magnet:?`),
  opción de categoría/carpeta de destino.
- **Endpoint**: `POST /torrents/add`.

---

## Feature 7 — Gestión de la máquina (`/manage`)

**Pantalla** con el estado de la Pi y los botones que actúan sobre ella:
reiniciar Jellyfin, qBittorrent, el túnel o Holocron; reiniciar la Pi; apagarla.

### Por qué no puede hacerlo Holocron solo

El servicio corre sin privilegios, con `NoNewPrivileges=true` y
`ProtectSystem=strict`. Eso **descarta `sudo` de entrada**: con esa bandera el
kernel ignora el bit setuid, así que `sudo` no funciona desde ese proceso ni
agregando al usuario a sudoers. Habría que aflojar el endurecimiento.

Se usa el mismo patrón que la actualización: Holocron deja un archivo, una
`.path` unit de root lo ve y actúa.

### Un trigger por acción, y vacío

Cada acción tiene **su propio archivo y su propio par de units**, y el archivo
no tiene contenido: es una señal, no un mensaje. El `ExecStart` de cada unit
queda fijo al instalar y nunca lee lo que Holocron escribió.

Poner el nombre de la acción **adentro** de un trigger compartido pondría a un
proceso root a parsear datos escritos por un servidor web, y la seguridad
pasaría a depender de una lista blanca en el código de Holocron — justo lo que
el patrón evita. Con un unit por acción, el conjunto de cosas que pueden pasar
**es** el conjunto de units instalados, auditable desde afuera con
`systemctl list-units 'holocron-*'`.

### El ciclo de dependencias que sólo aparece al reiniciar

La unit de limpieza necesita `DefaultDependencies=no`, `After=local-fs.target`,
**`Before=paths.target`** y **`WantedBy=sysinit.target`**. Con las dependencias
por defecto hereda `After=basic.target`, que corre *después* de `paths.target`;
como las `.path` dependen de ella, systemd encuentra un ciclo y lo rompe
**descartando las `.path`**.

Lo peligroso es cuándo se nota: en la sesión donde se instala funciona todo, y
recién al reiniciar aparecen todas las `.path` en `loaded / inactive (dead)` y
ningún botón anda. El chequeo barato tras un arranque es
`journalctl -b | grep -c 'Found dependency on'`, que tiene que dar 0.

### El bucle de apagado

Si un trigger de `poweroff` sobrevive a un reinicio —un corte de luz entre que
se escribe el archivo y que el `rm` llega al disco—, la `.path` unit lo ve al
arrancar y **la Pi se apaga sola cada vez que enciende**, recuperable sólo con
teclado y monitor. Por eso hay una oneshot al arranque
(`holocron-action-reset.service`) que borra los triggers residuales, ordenada
`Before=` todas las `.path`.

### Cómo probar el mecanismo sin apagar la máquina

**Escribir un disparador lo ejecuta en el acto.** Las `.path` están vigilando, así
que un `touch .poweroff-requested` apaga la Pi antes de que corra la línea
siguiente del procedimiento. Esto ya causó un apagado accidental de otro
disparador durante el desarrollo.

Para verificar la limpieza de disparadores hay que **parar antes las dos `.path`
peligrosas**, y el motivo va escrito al lado del paso porque un paso sin
explicación es el primero que alguien saltea por ir rápido:

```sh
# Sin esto, la línea siguiente apaga la máquina.
sudo systemctl stop holocron-poweroff.path holocron-reboot.path

sudo -u holocron touch /var/lib/holocron/.poweroff-requested
sudo systemctl restart holocron-action-reset.service
sudo ls -a /var/lib/holocron | grep -- '-requested'   # ls SIN -a no ve dotfiles

# Devolver la vigilancia, o los botones quedan muertos.
sudo systemctl start holocron-poweroff.path holocron-reboot.path
```

Dos trampas medidas: `ls` sin `-a` no lista archivos ocultos, así que una
verificación descuidada informa «0 residuales» sin haber mirado nada; y si el
nombre de prueba no termina exactamente en `-requested`, el glob no lo toca.

Para probar el mecanismo entero sin riesgo, usar `restart-jellyfin`: recorre el
mismo camino y lo peor que pasa es que se corte una reproducción.

### Fricción despareja, a propósito

Apagar es lo único que pide el **token de la API**; reiniciar no. La línea es la
irreversibilidad, no el trastorno: un Pi 4 no tiene wake-on-LAN, así que apagarlo
es de ida salvo que haya alguien al lado. Darle a reiniciar la misma fricción
entrenaría el mismo gesto para los dos, y el que muerde es el que no vuelve.

En iOS la misma idea toma otra forma, porque la app **tiene** el token guardado
y mandarlo sería fricción cero: apagar se confirma **manteniendo apretado**.

### Apagar desde afuera de casa

Se puede, y al principio no se podía. La primera versión lo **negaba** cuando la
dirección configurada no era de la red de casa, y era la decisión equivocada:
salir de casa es justamente cuando querés poder apagarla, y ni la app ni el
servidor saben si hay alguien adentro para volver a encenderla. Negarlo era
decidir por el usuario algo que sólo él puede saber.

Ahora la consecuencia se **enuncia y hay que aceptarla**, en las dos superficies:

- **Web**: aparece un `<input type="checkbox" name="ack" required>` arriba del
  campo del token. `required` lo hace cumplir el navegador sin una línea de
  JavaScript propio.
- **iOS**: un `Toggle` que **arma** el botón de mantener apretado. Dos gestos
  distintos, y el primero es el que lleva la frase.

El servidor lo revalida en `needsStrandAck` (`internal/httpserver/manage.go`),
para la web y para la API, de modo que las dos superficies no puedan divergir —
y así una página cargada en casa y enviada más tarde desde el tren igual pregunta.

**Es fricción, no autorización**, y el código lo dice: quien tiene el token puede
mandar `ack=1` a mano, y el `Host` del que se deduce «afuera» lo controla el
cliente. Lo que compra es que un apagado remoto sea una decisión y no un toque
mal dado.

La asimetría del riesgo es la que uno quiere, y está medida: por el túnel llega
`Host: holocron.merli.store` (cloudflared no reescribe) y por la LAN
`192.168.0.2:8090`. Desde la LAN se puede mentir el `Host` y hacerse pasar por
remoto, pero eso sólo se gana **más** fricción. Al revés no se puede: Cloudflare
enruta por ese mismo `Host`, así que un request de afuera que ponga
`Host: localhost` no llega nunca al túnel.

Un caso que conviene saber que es deliberado: **Tailscale cuenta como afuera**.
Su rango es 100.64/10 (CGNAT) y `netaddr.IsPrivateHost` no lo trata como
privado. Es una red privada pero no una *cercana* — llegar por la VPN desde otro
país se ve idéntico a llegar desde el sillón, que es el punto de Tailscale.

Por qué no hizo falta tocar Cloudflare: la app iOS pega a `/api/v1/manage/action`,
que cae bajo la aplicación de Access de `/api` (service token), no bajo la de
`/manage` (identidad de Google). El navegador entra por la de Google. Cada
superficie ya tenía su credencial.

### El chequeo previo

Antes de los botones, la pantalla dice qué se interrumpiría: quién está
reproduciendo qué, si Jellyfin tiene una tarea corriendo y cuántos torrents
están activos. Es la mitad del valor de la pantalla — la pregunta antes de
apagar nunca es «¿estás seguro?», es «¿hay alguien mirando algo?», y eso se
puede consultar.

Los sondeos son *best effort*: uno que no contesta no aporta advertencia en vez
de bloquear la página. `Checked` distingue **«no hay nada en curso»** de **«no
se pudo preguntar»**, que se ven iguales y sólo uno de los dos significa que es
seguro apagar.
