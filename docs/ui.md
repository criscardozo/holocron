# Guía de UI — tema «Noir»

Cómo está construida la interfaz de Holocron y qué reglas hay que respetar al
tocarla. La dirección visual es **Noir**: negro + naranja, tipografía de sistema,
íconos SVG de línea. El material de referencia del rediseño (prototipos HTML y
capturas) vive en [`design_handoff_holocron_noir/`](design_handoff_holocron_noir/).

## Restricciones (no negociables)

Condicionan cualquier cambio visual:

1. **Sin build step de front-end.** Un único archivo `web/static/styles.css`
   escrito a mano, embebido con `//go:embed`. No hay bundler, Sass ni Tailwind.
2. **Sin JavaScript propio.** La interactividad es HTMX (`hx-*`) más CSS
   (`transition`, `:hover`, `@keyframes`). No se agregan scripts.
3. **Todo self-contained.** Nada de CDNs, fuentes remotas ni imágenes externas.
   Por eso la tipografía es la stack del sistema y no *Archivo* (la fuente del
   diseño): embeber un `.woff2` sumaría peso al binario sin necesidad.
4. **CSP estricta: prohibido `style=""` inline.** Es la trampa más fácil de
   pisar (ver abajo).
5. **Corre en una Raspberry Pi.** Liviano es mejor.

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
--color-bg        #121110   fondo          --ok       #6bbf8f   sí / sembrando
--color-surface   #1a1817   tarjetas       --danger   #e08a8a   no / borrar / error
--color-surface-2 #221f1d   inputs/hover   --warn     var(--color-accent-300)
--color-text      #f1ede9   texto
--color-accent    #ff6a2b   naranja (barras, primary, marca)
--color-accent-200/300/400/800/900        escalones del acento
--color-divider   color-mix(… 12% …)
```

Espaciado `--space-1..8` (4→32 px), radios `--radius-sm/md/lg`, elevación
`--shadow-sm/md/lg` (hairline + sombra ambiente). Números siempre con
`font-variant-numeric: tabular-nums`; rutas y nombres de archivo en `.mono`.

## Componentes

En `web/templates/`. Los principales:

- **`Layout(title)`** — shell común: topbar sticky con blur, marca (diamante
  naranja), 7 links. El link activo se marca solo: se compara el label con el
  título de la página, así que **el título debe coincidir** con el label del nav
  (`Disco`, `Nombres`, `Medios`, `Subtítulos`, `Torrents`, `Ajustes`).
- **`Icon(id)` / `IconSm(id)`** — íconos del sprite SVG embebido en el layout.
  Para sumar uno, se agrega un `<symbol>` en `iconSprite()`. No se usan emojis.
- **`Widget(chrome, body)`** — tarjeta del dashboard. `WidgetChrome` lleva el
  ícono, el `Span` en la grilla (`span-2`, `span-4`) y `Attn` (borde de acento a
  la izquierda, para lo que requiere atención).
- **`JobStatus` / `ScanStatus`** — feedback de trabajos en background: spinner +
  polling cada 2 s, y al terminar recarga la sección con `hx-select`.
- Utilitarios: `.card`, `.btn` (`-primary` outline de acento, `-secondary`,
  `-ghost`, `-icon`, `-danger`), `.table`, `.input`, `.field`, `.badge`
  (`-yes/-no/-warn/-neutral`), `.st` (pills de torrent), `.notice`, `.tabs`,
  `.bar`/`.bar-fill` (+ `.hot`), `.stat-list`, `.stat-trio`, `.big-num`.

## Estados

El dashboard tiene que dejar leer de un vistazo qué necesita atención:

- **Con datos** — normal.
- **Necesita acción** — número grande en acento + `attn-widget` + un chip en la
  tira «Atención» del encabezado (nombres inválidos, subtítulos faltantes,
  discos ≥ 90 %).
- **No configurado** — texto apagado con link a Ajustes ("Jellyfin no configurado").
- **Error / sin conexión** — `.notice.error`, `.badge-no` o `.st-err`.

Lo que está bien queda tranquilo de fondo; lo que falta, salta.

## Responsive

Breakpoints de la grilla del dashboard y de Ajustes:

| Ancho | Efecto |
|---|---|
| ≤ 1100 px | la grilla pasa a 2 columnas (`span-4` sigue ocupando el ancho) |
| ≤ 860 px | `settings-grid` pasa a 1 columna |
| ≤ 680 px | la grilla pasa a 1 columna; el explorador oculta la mini-barra |

## Accesibilidad

- `:focus-visible` con anillo de acento (nunca quitarlo).
- `aria-label` en todo botón que sea solo ícono, **incluyendo a qué se refiere**:
  "Pausar `<nombre del torrent>`", "Abrir `<carpeta>`", no sólo "Pausar".
- Todo lo que sea accionable es `<button>` o `<a>` real, nunca un `<div>` con
  `hx-get`: las filas del explorador y de «carpetas más grandes» son botones para
  que se puedan recorrer con el teclado (`button.fs-row` neutraliza el estilo).
- `aria-current="page"` en el link activo (topbar y tabs de Disco), y las tabs
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
