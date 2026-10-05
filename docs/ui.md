# Guía de UI — el estilo de Ginebra

Cómo está construida la interfaz de Holocron y qué reglas hay que respetar al
tocarla. Desde octubre de 2026 la dirección visual es **la de Ginebra**, el
servidor donde corre: el mural de pósters de la propia biblioteca detrás,
tarjetas de vidrio con borde fino y sin sombras, acento violeta con el rosa y el
lila del ícono G▶, y la misma tipografía de sistema que la portada
(`servidor/web/index.html` en el repo de Ginebra). Holocron es la herramienta del servidor, no otro producto: su marca es el
holocrón, un cubo isométrico que dibujó Ginebra con los colores del G▶
(`web/static/holocron.svg`), con «Holocron» y la etiqueta «GINEBRA» en lila al
lado, que dice de qué servidor es.

Antes fue el tema «Noir» (negro + naranja). Su material de referencia sigue en
[`design_handoff_holocron_noir/`](design_handoff_holocron_noir/) como historia.

## Restricciones (no negociables)

Condicionan cualquier cambio visual:

1. **Sin build step de front-end.** Un único archivo `web/static/styles.css`
   escrito a mano, embebido con `//go:embed`. No hay bundler, Sass ni Tailwind.
2. **HTMX primero, JavaScript propio cuando un widget lo justifique.** La
   interactividad base es HTMX (`hx-*`), más su extensión SSE para lo que va en
   vivo, y CSS (`transition`, `:hover`, `@keyframes`). Se permite JavaScript
   propio desde octubre de 2026 (decisión de Cristian al pasar a Ginebra), con
   dos condiciones: va **siempre en un archivo `.js` externo y embebido**, nunca
   inline, para que la CSP `script-src 'self'` no cambie; y cada archivo
   resuelve algo que HTMX no puede, por ejemplo un gráfico interactivo. Si se
   juntan más de dos o tres, se ordenan en vez de acumularse.
3. **Todo self-contained.** Nada de CDNs, fuentes remotas ni imágenes externas.
   Por eso la tipografía es la stack del sistema y no *Archivo* (la fuente del
   diseño): embeber un `.woff2` sumaría peso al binario sin necesidad.
4. **CSP estricta: prohibido `style=""` inline.** Es la trampa más fácil de
   pisar (ver abajo).
5. **Las imágenes salen de Holocron.** Los pósters los sirve `/art/…` desde su
   caché en disco (ver «Pósters»); el navegador nunca le habla a Jellyfin ni a
   TMDb, y la CSP no cambia.

### La regla del `style` inline

La CSP es `default-src 'self'` sin `'unsafe-inline'`, así que **el navegador
descarta cualquier atributo `style`**. Un `style="width:63%"` no falla ruidosamente:
simplemente la barra no se dibuja.

Para valores dinámicos se usan clases precomputadas:

| Necesidad | Solución | Helper |
|---|---|---|
| Ancho de barra 0–100 % | `.pw-0` … `.pw-100` | `pwClass(pct)` |
| Ancho fijo de columna | `.w-70`, `.w-80`, `.w-90`, `.w-110`, `.w-120`, `.w-130`, `.w-190` | — |
| Margen/alineación puntual | `.mb-6`, `.ml-auto`, `.list-foot` | — |

Si hace falta un ancho nuevo, se agrega la clase al CSS; no se vuelve al `style`.

**htmx también pisa esta regla.** Por defecto inyecta su CSS de indicadores
como un `<style>` inline, que la CSP descarta (`Applying inline style violates…`
en la consola) y deja los `hx-indicator` sin efecto. Por eso el layout lleva
`<meta name="htmx-config" content='{"includeIndicatorStyles":false}'>` —por meta
y no por script, que también estaría bloqueado— y las reglas `.htmx-indicator`
viven en `styles.css`.

### Cuidado con `.card` y flex

`.card` fija `flex-direction: column`. Un modificador que quiera una fila sobre el
**mismo** elemento (`class="card … stat-band"`) tiene que declarar
`flex-direction: row` explícitamente, si no hereda la columna y el contenido se
apila. Le pasa a `.stat-band` y a `.magnet-form`.

### Tablas anchas

Van envueltas en `.table-scroll`, que hace `overflow-x: auto` con
`min-width: 720px` en la tabla: scrollean dentro de su caja en vez de ensanchar
la página. Se usa en el inventario de Medios y en la tabla de Torrents.

## Tokens

Definidos como custom properties en `:root`. Usar siempre las variables.

```
--color-bg        #07090e   fondo (el de la portada)   --ok      #34d399
--color-surface   #13171f   tarjetas                   --danger  #f43f5e
--color-surface-2 #1d2432   inputs / hover             --warn    #facc15
--color-text      #eef1f6   texto
--color-accent    #a78bfa   violeta claro (barras, primary, foco)
--color-accent-300 #c084fc  lila (etiquetas, «GINEBRA»)
--color-accent-400 #7c3aed  violeta
--color-pink      #f472b6   rosa (segunda serie, degradé de la pestaña activa)
--color-divider   rgba(255,255,255,.09)  el borde fino de todo
```

Las tarjetas son las baldosas de la portada: `--glass` (`rgba(24,29,40,.72)`)
con `backdrop-filter: blur(14px)` sobre el mural, borde `--glass-border` y radio
18 px. `--color-surface` queda para lo que va adentro de una tarjeta (inputs,
botones), donde el vidrio sobre vidrio no se distingue.

Espaciado `--space-1..8` (4→32 px), radios `--radius-sm/md/lg/xl` (6 / 12 / 16 /
18 px, como los paneles y las baldosas de la portada). **Sin sombras**: `--shadow-*` es sólo el borde
fino, como en la portada, donde la profundidad sale del borde y no de sombras. Números siempre con
`font-variant-numeric: tabular-nums`; rutas y nombres de archivo en `.mono`.

## Componentes

En `web/templates/`. Los principales:

- **`Layout(title)`** — shell común: el mural, y una barra lateral con la marca
  y el menú en cuatro grupos (Ahora, Biblioteca, Descargas, Equipo; `navGroups`
  en `components.go`). El link activo se marca solo: se compara el label con el
  título de la página, así que **el título debe coincidir** con el label del
  menú (`Inicio`, `Actividad`, `Hardware`, …, `Ajustes`).
- **El mural** — `muralWall`, detrás de todo: 28 pósters de la biblioteca
  sorteados cada 15 minutos y repetidos para llenar la pantalla, atenuados por
  `.velo`. Lo pone el middleware `withMural` en el contexto de cada página, así
  que ningún handler lo pasa a mano. Lleva `hx-preserve` para que la navegación
  con `hx-boost` no lo recargue. Opacidad `.34` en Inicio y `.16` en el resto:
  detrás de un número de trabajo tiene que quedar de fondo.
- **`Icon(id)` / `IconSm(id)`** — íconos del sprite SVG embebido en el layout.
  Para sumar uno, se agrega un `<symbol>` en `iconSprite()`. No se usan emojis.
- **`tile(Tile)`** — baldosa de Inicio, una por área: ícono en degradé
  (`tone-*`), título, un número grande y una línea. Es el link a su pantalla, y
  el número tiene que decir si vale la pena entrar. Las arma `widgets`.
- **`poster(src, title)`, `posterRow`, `.poster-grid`** — pósters: fila con
  scroll horizontal (Agregado hace poco) o grilla (Medios), siempre con
  `loading="lazy"`. Sin imagen, la inicial del título sobre un degradé.
- **`kpi(...)`** — número de cabecera con icono y aclaración (Hardware).
- **`JobStatus` / `ScanStatus`** — feedback de trabajos en background: spinner +
  polling cada 2 s, y al terminar recarga la sección con `hx-select`.
- Utilitarios: `.card`, `.btn` (`-primary` outline de acento, `-secondary`,
  `-ghost`, `-icon`, `-danger`), `.table`, `.input`, `.field`, `.badge`
  (`-yes/-no/-warn/-neutral`), `.st` (pills de torrent), `.notice`, `.tabs`,
  `.bar`/`.bar-fill` (+ `.hot`), `.stat-list`, `.stat-trio`, `.glass-chip`,
  `.kicker`.

## Pósters

Los sirve `GET /art/{kind}/{id}`: `jf/<id de Jellyfin>` o `tmdb/<archivo>` para
lo pedido en Seerr que todavía no está en la biblioteca. El paquete `artwork` los
baja **una vez**, ya escalados a 300 px, y los guarda en `artwork/` junto a la
base; después salen del disco. Reglas:

- **El id se valida antes de tocar nada**: 32 hexadecimales para Jellyfin, un
  nombre de archivo simple para TMDb. Cualquier otra cosa es 404.
- **Un póster que no existe es un SVG en blanco, no un 404.** Un `<img>` que
  falla muestra el ícono de imagen rota, y reemplazarlo pediría un `onerror` que
  la CSP no deja.
- Se renuevan a los 30 días; si la fuente no responde, se sigue sirviendo el
  viejo. Lo que falta se recuerda una hora, para no preguntarlo en cada página.
- Para un episodio se usa el póster de la serie: el del episodio es un fotograma.

## Estados

Inicio tiene que dejar leer de un vistazo qué necesita atención:

- **Con datos** — normal.
- **Necesita acción** — el número de la baldosa en lila (`tile-warn`) y un chip
  en la tira «Atención» (un corte de luz, nombres inválidos, discos ≥ 90 %).
- **No configurado** — texto apagado con link a Ajustes ("Jellyfin no configurado").
- **Error / sin conexión** — `.notice.error`, `.badge-no` o `.st-err`.

Lo que está bien queda tranquilo de fondo; lo que falta, salta.

## Responsive

Las baldosas, los pósters y las tarjetas de Actividad y Hardware se acomodan
solos (`auto-fill` / `auto-fit`). Breakpoints fijos:

| Ancho | Efecto |
|---|---|
| ≤ 900 px | la barra lateral pasa a ser una barra arriba que scrollea de costado; la cabecera de Actividad se achica |
| ≤ 860 px | `settings-grid` pasa a 1 columna |
| ≤ 680 px | el explorador oculta la mini-barra |

## Accesibilidad

- `:focus-visible` con anillo de acento (nunca quitarlo).
- `aria-label` en todo botón que sea solo ícono, **incluyendo a qué se refiere**:
  "Pausar `<nombre del torrent>`", "Abrir `<carpeta>`", no sólo "Pausar".
- Todo lo que sea accionable es `<button>` o `<a>` real, nunca un `<div>` con
  `hx-get`: las filas del explorador y de «carpetas más grandes» son botones para
  que se puedan recorrer con el teclado (`button.fs-row` neutraliza el estilo).
- `aria-current="page"` en el link activo (menú lateral y tabs de Disco), y las tabs
  van en un `<nav aria-label="Carpetas vigiladas">`.
- Contraste suficiente sobre el fondo oscuro: para texto de párrafo en acento
  usar `--color-accent-300`, no el acento puro.

## Al tocar la UI

```sh
go tool templ generate   # tras editar cualquier .templ
make run                 # local en :8090
```

Y antes de commitear, la verificación que más veces salvó esto:

```sh
curl -s localhost:8090/ | grep -o 'style="[^"]*"'   # no debe imprimir nada
```
