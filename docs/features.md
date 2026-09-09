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

### Una acción sólo existe si su servicio existe

La cuarta columna de `power_actions` en el instalador nombra la unit que tiene
que estar **enabled** para que el control se instale. Vacía significa siempre:
reiniciar y apagar no tienen un servicio detrás, y `holocron` es la unit que el
propio script acaba de instalar.

No es comodidad, es seguridad, y se descubrió midiendo:
**`systemctl restart` arranca una unit deshabilitada** — `disabled` gobierna el
arranque del sistema, no el start manual. Así que un botón para un servicio que
alguien apagó a propósito es un botón que lo vuelve a encender. Pasó con
`cloudflared` después de dar de baja el acceso público: la sesión ObiWan lo
probó y el proceso levantó y se reconectó al edge de Cloudflare.

Se mira `is-enabled` y **no** `is-active`: lo segundo sacaría un botón porque el
servicio justo se estaba reiniciando, y lo devolvería después, que es peor que
cualquiera de las dos respuestas por separado.

El bucle de instalación recorre la lista **completa** y no la filtrada, porque
tiene que **borrar** lo que ya no corresponde además de escribir lo que sí. Una
corrida que sólo crea deja la decisión de ayer en disco, que es exactamente cómo
el botón de cloudflared volvería en el próximo update. Y el `Before=` de la unit
de reset se genera desde la lista filtrada: si no, systemd queda sosteniendo una
referencia `not-found` y el próximo que mire el listado va a buscar un problema
que no existe.

Todo esto tiene un test que corre con `go test ./...`
(`scripts/install_power_test.sh`, invocado desde `internal/power`). Es la
primera parte del instalador con cobertura, y no por casualidad: es la que
deshizo en silencio una decisión deliberada **dos veces** —primero
`ReadWritePaths`, después este botón— y leerla no alcanzó ninguna de las dos.

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

### Cómo se decide «afuera», y qué cambió cuando se bajó Cloudflare

Se lee del `Host` que recibe el origen. Hoy:

| Entrada | `Host` | ¿Pide confirmación? |
|---|---|---|
| LAN directa | `192.168.0.2:8090` | no |
| Tailscale, por IP | `100.94.171.18:8090` | **sí** |
| Tailscale, por nombre | `obiwan.ayu-palermo.ts.net:8090` | **sí** |

**Tailscale cuenta como afuera, y es deliberado.** Su rango es 100.64/10
(CGNAT) y `netaddr.IsPrivateHost` no lo trata como privado. Es una red privada
pero no una *cercana*: llegar por la VPN desde otro país se ve idéntico a llegar
desde el sillón, que es exactamente el punto de Tailscale. Como el acceso remoto
ahora va todo por ahí, en la práctica la confirmación se pide siempre salvo
desde la LAN.

**Una versión anterior de esta sección afirmaba una asimetría que ya no
existe**, y vale dejar registrado por qué. Decía: desde la LAN se puede mentir
el `Host` y hacerse pasar por remoto, pero eso sólo gana más fricción; y al
revés no se puede, porque Cloudflare enruta por ese mismo `Host` y un request de
afuera con `Host: localhost` nunca llegaría al túnel.

La primera mitad sigue siendo cierta. **La segunda dependía de que hubiera un
túnel delante**, y en septiembre de 2026 se dio de baja el acceso público
—cloudflared parado, los CNAME borrados— y todo el acceso remoto pasó a
Tailscale. Ya no hay nada que enrute por `Host`: cualquiera que alcance el
puerto 8090 puede mandar el que quiera, incluido uno privado, y saltear la
confirmación.

Eso no cambia el diseño, porque **nunca fue autorización**: quien tiene el token
puede mandar `ack=1` a mano igual. Pero una propiedad de seguridad escrita en la
documentación sobrevive a la infraestructura que la sostenía, y esta ya no
aplica.

Lo que sí cambió de verdad: **`/api` quedó con una sola capa.** Antes tenía
Cloudflare Access con service token por delante y el bearer de Holocron por
detrás. Ahora la frontera es la red de Tailscale más el bearer. La consecuencia
práctica es que el token de la API dejó de ser el segundo factor y pasó a ser el
único, y quien esté en el tailnet lo tiene todo salvo ese token.

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

### «Fantasma» y «todavía no salió» no son lo mismo

Un episodio que Jellyfin lista sin archivo puede ser dos cosas opuestas, y
durante un tiempo el panel las contó juntas bajo **Fantasmas**, con el consejo
de limpiarlas borrándolas desde Jellyfin.

Para una temporada en curso eso significaba **borrar la temporada que estás
mirando**. Jellyfin conoce los episodios futuros por la metadata del proveedor y
los lista sin archivo, que es exactamente lo que parece un archivo borrado.

Se descubrió midiendo la biblioteca real: de los 11 «fantasmas» de una serie,
**6 eran episodios sin emitir** y el primero salía al día siguiente. Los otros
~93 sí eran temporadas ausentes de verdad.

Ahora se distinguen por `PremiereDate`, que se pide en `auditFields`
justamente para esto. Fecha futura → **Todavía no salieron**, con el consejo
inverso: no los toques. Fecha pasada, o sin archivo y sin fecha → Fantasma.

**Sin fecha cuenta como emitido**, no como futuro. Mucho material viejo no
tiene `PremiereDate`, y leer «desconocido» como «va a salir» escondería un
archivo faltante real detrás de una etiqueta tranquilizadora — el mismo error
en la otra dirección.

`Analyse` toma el instante como parámetro en vez de llamar a `time.Now()`
adentro, así el límite que decide entre «borrá esto» y «no toques esto» se
puede testear en un momento fijo.

## Feature 8 — Renombrado masivo de películas (`/naming/rename`)

Lleva las carpetas de películas a «Título (Año)» y arrastra con ellas todo lo
que dependía del nombre viejo.

### Por qué es una pantalla aparte y no un botón en `/naming`

Es lo más destructivo que hace Holocron: cambia archivos que no se pueden
reconstruir, en masa, en un disco compartido con un servidor de medios. La
vista previa **es** la feature. Nada pasa hasta que la lista exacta de cambios
estuvo en pantalla — cada carpeta y cada archivo adentro — y cada carpeta tiene
su tilde para dejarla afuera.

### Nada se renombra solo

La unidad de trabajo es la carpeta, no el archivo. Un subtítulo se asocia a su
video por el nombre, y lo mismo vale para las imágenes que Jellyfin levanta por
nombre de archivo. Renombrar la carpeta y dejar `Movie.1080p.es.srt` adentro
sale gratis en el momento y cuesta los subtítulos para siempre, sin un solo
mensaje de error: la película simplemente se reproduce sin ellos.

Qué se mueve junto: video, subtítulos (`.srt`, `.ssa`, `.ass`, `.sub`, `.idx`,
`.vtt`…), imágenes que llevan el nombre de la película y `.nfo`. Qué **no** se
toca: `poster.jpg`, `fanart.jpg`, `logo.png` — esos ya son los nombres que
Jellyfin busca, y reescribirlos rompería lo que funciona.

El «mismo nombre» se resuelve contra dos candidatos: el nombre de la carpeta y
el de cualquier video adentro, el más largo primero. Eso cubre la carpeta cuyo
contenido no coincide con ella, que es la mitad de una biblioteca armada
durante años. El prefijo tiene que terminar en un separador, si no `Alien 2`
haría match adentro de `Alien 2049`.

### Nada se pisa, nunca

Todo renombrado cuyo destino ya existe se saltea y se informa. En una
biblioteca una colisión significa dos películas distintas, no una copia vieja
— y `rename(2)` reemplaza el destino sin decir una palabra, así que un
renombrado descuidado no genera desorden: borra una película.

El chequeo es `Lstat` y después `Rename`. No es hermético —nada impide que el
destino aparezca en el medio— pero la alternativa que sí lo sería, un `link` +
`unlink`, necesita un filesystem con enlaces duros y la biblioteca está en
exFAT. Cierra el caso que pasa, no la carrera.

### El parser de títulos

`naming.Parse` es la parte que tenía que estar bien. La sugerencia anterior se
mostraba al lado de la carpeta y la leía una persona, que notaba si era
disparate. Ahora maneja un renombrado masivo, y una respuesta plausible pero
equivocada es peor que una obviamente equivocada: nadie revisa un nombre que se
ve bien, y para cuando el scraper matchea la película equivocada el archivo ya
está renombrado.

Lo que la versión vieja hacía mal, medido:

| Carpeta | Antes | Ahora |
|---|---|---|
| `The.Matrix.1999.1080p.BluRay` | `The.Matrix..1080p.BluRay (1999)` | `The Matrix (1999)` |
| `Blade.Runner.2049.2017` | `Blade.Runner..2017 (2049)` | `Blade Runner 2049 (2017)` |
| `2012.2009` | `2009 (2012)` | `2012 (2009)` |

El año se toma del **último** token que parece año, no del primero: los títulos
que contienen un año lo ponen antes del de estreno (`Blade Runner 2049 2017`,
`2012 2009`, `1917 2019`) y tomar el primero los rompe a todos.

Los tags de release (`1080p`, `WEB-DL`, `x265`, `REMUX`…) sólo se sacan del
**final** del título, así que una película llamada `Dual` o `Cam` conserva su
nombre. Los puntos se convierten en espacios sólo cuando hay más puntos que
espacios, así `Mr. Nobody (2009)` no pierde el suyo.

### No se inventa el año

Una carpeta sin año no se toca y aparece en su propia lista. El año es lo que
distingue dos películas con el mismo título; adivinarlo manda al scraper a la
equivocada detrás de un nombre que parece deliberado. En esta biblioteca son
sobre todo títulos argentinos y mexicanos viejos (`Esperando la carroza`,
`Cien veces no debo`).

### Carpetas que no son películas

Dos mecanismos, porque hay dos clases de caso.

Las **carpetas ocultas se saltean solas**: nada cuyo nombre empiece con punto es
una película, así que no hay nada que decidir. Eso cubre `.claude` y el estado
de trabajo de cualquier herramienta, más `.Spotlight-V100` y `.Trashes`, que
esta biblioteca junta por vivir en exFAT y ser tocada desde una Mac.
`System Volume Information` es lo mismo desde Windows y no es oculta por su
nombre, así que está nombrada aparte.

Todo lo demás **sí es un juicio, y es del usuario**: cada fila de `/naming`
tiene un botón «Ignorar». La lista de ignoradas se muestra abajo con un botón
para volver atrás — un ignorar de una sola dirección achica la lista en silencio
hasta que nadie recuerda qué falta en ella.

El ignorar vive en su propia tabla y no en un campo de `naming_issues`, porque
cada escaneo borra y reescribe esa tabla: una marca ahí se perdería la próxima
vez que alguien apretara refrescar, que es justo cuando importa.

Se chequea en tres lugares —al escanear, al armar la vista previa y **otra vez**
al aplicar— porque la vista previa y el apply son requests distintos, y un
ignorar agregado en el medio tiene que ganar. Si no, el único caso para el que
existe el botón es el que se le escapa.

### Sólo películas

Las series quedan afuera a propósito. Los episodios llevan número de temporada
y de capítulo, que este parser no conoce, y aplicarles las mismas reglas
convertiría una biblioteca que funciona en una pila de archivos con el nombre
de la serie.

### Confinamiento

Todo pasa por `os.Root` abierto en la carpeta de medios configurada. Las claves
que vuelven del formulario son entrada del usuario camino a un renombrado, así
que `locate` exige que la ruta esté **un** nivel adentro de una carpeta
configurada: la carpeta de medios en sí, algo más profundo, un hermano con
prefijo parecido (`/mnt/Peliculas-viejas`) o cualquier `..` se rechazan y se
informan.

El plan se **recalcula** al aplicar en vez de arrastrarse desde la vista
previa. El disco pudo cambiar en el medio —hay un servidor de medios
escribiendo— y actuar sobre un plan viejo es como un renombrado termina sobre
un nombre que ahora pertenece a otra cosa. La vista previa es orientativa; el
apply es la decisión.

## Feature 9 — Trailers (`/trailers`)

Busca en YouTube el trailer de las películas que no tienen uno y lo baja al
lado del archivo.

### La convención es la que hay, no la que uno supone

Jellyfin reconoce un trailer por el sufijo `-trailer` antes de la extensión, y
es lo que esta biblioteca ya usa: **`<nombre>-trailer.mp4` al lado de la
película**, no `<nombre>.trailer.mp4` ni una subcarpeta `trailers/`. Medido
sobre la biblioteca real.

El archivo que se escribe se llama **como la carpeta**, nunca como el video de
YouTube. La biblioteca ya arrastra cosas como
`A MAN CALLED OTTO  Trailer oficial  Subtitulos Español Latinoamericano-trailer.mp4`
de haberlo hecho al revés, y reintroducir eso en el mismo release que limpia
los nombres sería absurdo.

### Elegir el video es la feature

Una búsqueda de «`<película>` trailer» devuelve el trailer, y también
reacciones, análisis de veinte minutos, fan edits, clips y a veces la película
entera. El script anterior tomaba el primer resultado, así que lo que YouTube
hubiera decidido rankear primero entraba a la biblioteca bajo un nombre que
dice «trailer», y nadie se enteraba hasta darle play.

Se descarta lo que no es un trailer: duración fuera de 20 s – 10 min, y títulos
con reaction / reseña / breakdown / explicado / recap / making of / fan made /
película completa / soundtrack. Se puntúa lo que queda por: decir «trailer»,
decir «oficial», durar entre 55 s y 3 min 40 s, y cuánto del título de la
película aparece en el del video.

**Cero coincidencia de título es rechazo, no penalización.** Un video puede
parecer un trailer perfecto —oficial, la duración justa, dice trailer— y ser
otra película que la búsqueda trajo de casualidad. Esos puntúan bien en todo lo
demás, así que sólo esto los frena.

### Idioma original primero

El orden de búsqueda es la preferencia, escrita:

1. `"Título" AÑO official trailer` — el corte original
2. `Título AÑO trailer subtitulado español` — audio original, subtítulos
3. `Título AÑO trailer` — lo que haya

Es al revés del script anterior, que buscaba «subtitulado» y después «español»,
poniendo el doblaje adelante. Un título doblado (`doblado`, `español latino`,
`castellano`) pierde 4 puntos pero **no** se rechaza: para algunas películas
viejas es lo único que hay, y un trailer doblado es mejor que ninguno.

Las búsquedas se prueban en orden y la primera respuesta suficientemente buena
gana, en vez de correr las tres y quedarse con la mejor. Cada una es una
llamada de red, y con 181 películas la diferencia son minutos contra una hora.

### Verificado contra YouTube de verdad

Los tests unitarios sólo prueban que el puntaje se comporta con los ejemplos
que escribí, y los escribió la misma persona que decidió qué es un buen
candidato. `TestAgainstRealYouTube` (opt-in con `HOLOCRON_LIVE_YOUTUBE=1`)
corre la búsqueda completa contra la red. La última corrida acertó el trailer
oficial en 6 de 6, incluidas las cuatro películas argentinas y mexicanas viejas
que se esperaba que fueran las difíciles.

### El piso de resolución

Filtrar por duración y por palabras del título deja pasar un caso que no se ve:
un trailer legítimo con resolución basura. La biblioteca ya tiene
`Antes de amanecer` a **450x360** y `Antes del atardecer` a **320x240** — pasan
todos los filtros y no sirven en un televisor.

No se resuelve durante la búsqueda porque `--flat-playlist` no devuelve la
resolución. El piso va en el selector de formato de la descarga, y cuando nada
lo alcanza yt-dlp contesta `Requested format is not available` — eso se traduce
a un error propio y el llamador prueba el candidato siguiente.

Sobre el costo, con una corrección que vale anotar. La primera versión de esto
decía «quince veces más lento, hora y media sobre la biblioteca», medido en la
MacBook. En la Pi, que es donde corre, la sesión ObiWan midió otra cosa:

| | Mac | Pi 4 |
|---|---|---|
| búsqueda flat | 1,6 s | 6 s |
| búsqueda completa | 24,3 s | 14 s |
| factor | 15x | **2,3x** |

O sea que el argumento del costo **no se sostiene en el destino**: 42 minutos
contra 18 es una diferencia que esta feature podría pagar. Los tiempos de red y
CPU de una laptop con fibra no se transfieren a un Pi 4.

La razón real del diseño no es la velocidad sino la estructura: la descarga
puede rechazar un candidato que la búsqueda aprobó —por resolución o porque
YouTube dio de baja el video— así que el llamador necesita a dónde ir después,
con o sin resolución en la búsqueda.

Por eso `Find` devuelve una **lista ordenada** y no una respuesta: lo que
descalifica a un video —resolución, o que YouTube lo haya dado de baja— sólo se
descubre al intentar bajarlo, y ninguna de las dos cosas dice nada del
siguiente.

**Dos pisos, no uno.** Un piso único no puede estar bien acá: alto excluye los
trailers viejos que sólo existieron en 360p, que son justo las películas para
las que existe la feature; bajo deja pasar el 450x360. Entonces se ofrece
**todos** los candidatos a 480p primero, y sólo si ninguno llega se hace una
segunda pasada aceptando 360p. Así un 1080p le gana a un 360p aunque el 360p
tenga mejor título — que es exactamente lo que pasa buscando «Antes de
amanecer».

### La dependencia externa, y que se va a romper

Holocron es un binario estático que no asume toolchain en el destino, y esta es
la única parte que rompe la regla. Leer YouTube en Go puro no es algo que se
mantenga funcionando: los extractores cambian lo bastante seguido como para que
yt-dlp exista como proyecto de tiempo completo dedicado a seguirlos.

Consecuencia de diseño: **yt-dlp se trata como ausente por defecto, nunca como
un hecho.** La pantalla distingue «no está instalado», «está viejo» y «YouTube
cambió otra vez» porque tienen tres arreglos distintos, y un error genérico
manda a leer los logs de Holocron por un problema que no es de Holocron. Un
extractor roto corta el lote entero en vez de producir 181 fallas idénticas.

### El piso de disco

Antes de **cada** descarga, no una vez al empezar, se chequea que queden más de
20 GB libres. No es una estimación de cuánto pesan los trailers —promedian
28 MB, unos 5 GB en total— es un piso bajo el disco mismo: el volumen estaba al
97% y perdiendo 25 GB por día mientras esto se escribía, y hay otra cosa
escribiendo ahí. Una feature que escribe en ese disco tiene que frenar sola.

### Confinamiento

Las carpetas que llegan del formulario se comparan contra el último escaneo, no
se resuelven como rutas. Una carpeta que Holocron no encontró él mismo leyendo
una carpeta de medios configurada es una carpeta en la que no va a escribir, así
que no hay aritmética de rutas entre un request y una descarga.
