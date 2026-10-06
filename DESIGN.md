---
name: Holocron
description: El tablero de Ginebra, el servidor de medios de la casa.
colors:
  violeta-ginebra: "#a78bfa"
  lila-claro: "#ddd6fe"
  lila-faceta: "#c084fc"
  violeta-profundo: "#7c3aed"
  violeta-noche: "#2a1d4a"
  violeta-sombra: "#1a1430"
  rosa-faceta: "#f472b6"
  fondo-cuarto: "#07090e"
  superficie: "#13171f"
  superficie-alta: "#1d2432"
  texto: "#eef1f6"
  borde-fino: "rgba(255, 255, 255, 0.09)"
  vidrio: "rgba(24, 29, 40, 0.72)"
  vidrio-encendido: "rgba(32, 39, 54, 0.9)"
  luz-ok: "#34d399"
  luz-alarma: "#f43f5e"
  luz-aviso: "#facc15"
  luz-alarma-texto: "#fb7185"
  cielo: "#0ea5e9"
  indigo: "#4f46e5"
  brasa: "#f97316"
  texto-suave: "color-mix(in srgb, #eef1f6 65%, transparent)"
  texto-tenue: "color-mix(in srgb, #eef1f6 55%, transparent)"
typography:
  display:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "44px"
    fontWeight: 800
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "30px"
    fontWeight: 600
    lineHeight: 1.15
  numeral:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "24px"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "-0.02em"
    fontFeature: "tnum"
  title:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "15px"
    fontWeight: 600
    lineHeight: 1.15
  body:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.55
  label:
    fontFamily: "-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif"
    fontSize: "11px"
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: "0.14em"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "12.5px"
    fontWeight: 400
    lineHeight: 1.4
rounded:
  sm: "6px"
  md: "12px"
  lg: "16px"
  xl: "18px"
  pill: "999px"
spacing:
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "6": "24px"
  "8": "32px"
components:
  button-primary:
    backgroundColor: "transparent"
    textColor: "{colors.violeta-ginebra}"
    typography: "{typography.title}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.texto}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
  button-danger:
    backgroundColor: "transparent"
    textColor: "{colors.luz-alarma}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
  input:
    backgroundColor: "{colors.superficie}"
    textColor: "{colors.texto}"
    rounded: "{rounded.md}"
    padding: "6px 10px"
    height: "36px"
  card:
    backgroundColor: "{colors.vidrio}"
    textColor: "{colors.texto}"
    rounded: "{rounded.xl}"
    padding: "16px"
  tile:
    backgroundColor: "{colors.vidrio}"
    textColor: "{colors.texto}"
    rounded: "{rounded.xl}"
    padding: "18px"
  tile-hover:
    backgroundColor: "{colors.vidrio-encendido}"
  nav-link:
    backgroundColor: "transparent"
    textColor: "{colors.texto}"
    rounded: "{rounded.md}"
    padding: "8px 10px"
  nav-link-active:
    backgroundColor: "{colors.violeta-noche}"
    textColor: "{colors.texto}"
  chip-attention:
    backgroundColor: "{colors.violeta-sombra}"
    textColor: "{colors.lila-claro}"
    rounded: "{rounded.pill}"
    padding: "5px 11px"
  badge-ok:
    backgroundColor: "rgba(52, 211, 153, 0.16)"
    textColor: "{colors.luz-ok}"
    rounded: "{rounded.pill}"
    padding: "3px 9px"
  badge-alarm:
    backgroundColor: "rgba(244, 63, 94, 0.15)"
    textColor: "{colors.luz-alarma}"
    rounded: "{rounded.pill}"
    padding: "3px 9px"
---

# Design System: Holocron

## Overview

**Creative North Star: "El tablero del cuarto de máquinas"**

Holocron es el tablero de Ginebra: instrumentos precisos sobre un cuarto a oscuras,
cada número en su lugar y todo en calma hasta que algo pide atención. La biblioteca
de la casa está en las paredes, como un mural de pósters atenuado detrás de cada
pantalla. Lo que se lee va adelante, en paneles de vidrio. El cuarto es de Ginebra:
el mismo fondo casi negro, el mismo vidrio y la misma luz violeta y rosa que la
portada del servidor, porque Holocron es su herramienta y no otro producto.

La densidad es la de un instrumento, no la de un informe. Cada área se resume en un
número grande con una línea que lo explica, y el detalle está a un toque. El color
es escaso a propósito: el violeta marca lo que se toca y lo que está activo, y las
luces de estado (verde, rojo, amarillo) aparecen sólo cuando dicen algo. Lo que
está bien no tiene color de más.

Holocron no es un panel de administración genérico: nada de grillas de tarjetas
grises iguales ni tablas por todos lados. Si una pantalla empieza a parecerse a
eso, le falta un número que mande o le sobra una tabla.

**Key Characteristics:**
- Fondo casi negro con el mural de pósters de la biblioteca, atenuado detrás de un velo.
- Paneles de vidrio translúcido con borde fino; ninguna sombra.
- Un solo acento, el violeta de Ginebra, usado poco y siempre con intención.
- Números grandes y tabulares como protagonistas de cada panel.
- Componentes de contorno fino: la acción se nota sin gritar.
- La tipografía del sistema, como en la portada; ningún webfont.

## Colors

Una paleta de cuarto oscuro: neutros fríos casi negros, un acento violeta con sus
facetas lila y rosa, y tres luces de estado que se encienden sólo cuando hace falta.

### Primary
- **Violeta Ginebra** (#a78bfa): el acento de la portada de Ginebra. Botones
  principales (en el borde y el texto, nunca de relleno), el link activo del menú,
  las barras de progreso, el foco del teclado y el número de lo que pide atención.
- **Lila faceta** (#c084fc): la segunda luz del acento. La etiqueta «GINEBRA», los
  textos de párrafo que van en acento (más legibles que el violeta puro) y los
  avisos que no son alarma.
- **Violeta profundo** (#7c3aed): el extremo oscuro de los degradados de los íconos
  y de las barras «calientes».

### Secondary
- **Rosa faceta** (#f472b6): la cara de arriba del cubo y del ▶. Segunda serie en
  los gráficos (la red) y el otro extremo de los degradados de íconos. Nunca solo
  como color de una acción.

### Tertiary
- **Lila claro** (#ddd6fe): texto sobre los fondos violeta oscuro (chips de
  atención, avisos).
- **Violeta noche** (#2a1d4a) y **Violeta sombra** (#1a1430): los fondos de los
  chips de atención y de los avisos; el violeta apagado que deja leer el lila claro.

### Neutral
- **Fondo del cuarto** (#07090e): el fondo de toda la app, el de la portada.
- **Superficie** (#13171f): lo que va dentro de un panel: campos, botones
  secundarios, filas.
- **Superficie alta** (#1d2432): placeholders de pósters, barras de trabajos en
  curso, el hover de lo que ya es superficie.
- **Texto** (#eef1f6): todo el texto. Los secundarios son este mismo color con
  transparencia, en dos pasos y no en opacidades sueltas: **texto suave** (65 %)
  para rótulos y claves, y **texto tenue** (55 %) para lo que acompaña a un dato.
  Los dos pasan AA (7,5:1 y 5,5:1).
- **Borde fino** (blanco al 9 %): el borde de todo panel, campo y botón secundario.
- **Vidrio** (rgba(24, 29, 40, .72)) y **Vidrio encendido** (rgba(32, 39, 54, .9)):
  los paneles sobre el mural, en reposo y al pasar el mouse.

### Luces de estado
- **Luz ok** (#34d399): en línea, todo arriba, disponible.
- **Luz de alarma** (#f43f5e): caído, falló, sin luz, borrar. Sobre su propio
  fondo rojo, el texto chico va en **alarma para texto** (#fb7185), que llega a
  5,7:1 donde el rojo base no pasa de 4,2:1.
- **Luz de aviso** (#facc15): temperatura alta y lo que conviene mirar.

### Tonos de ícono
- **Cielo** (#0ea5e9), **Índigo** (#4f46e5) y **Brasa** (#f97316): sólo el segundo
  extremo de los degradados de los íconos de las baldosas, junto a una faceta del
  G▶. Nunca texto ni acción.

La página declara el esquema oscuro (`color-scheme: dark`), así que los controles
que dibuja el navegador (listas desplegables, barras de scroll, autocompletado)
salen oscuros, y el texto guía de los campos usa el texto tenue.

### Named Rules
**The One Voice Rule.** El violeta Ginebra es el único acento de interacción. El
rosa acompaña y el lila matiza, pero ninguno de los dos marca una acción.

**The Quiet-Until-Needed Rule.** Las luces de estado se encienden sólo cuando dicen
algo. Un servicio que anda no lleva verde de fondo; uno caído sí lleva rojo.

## Typography

**Display Font:** la del sistema (-apple-system, BlinkMacSystemFont, Segoe UI, Roboto, sans-serif)
**Body Font:** la misma
**Label/Mono Font:** ui-monospace, SFMono-Regular, Menlo, Consolas, monospace

**Character:** una sola familia, la del sistema, como en la portada de Ginebra:
neutra y nítida, para que manden los números. La jerarquía sale del tamaño y del
peso, no de una segunda fuente. La monoespaciada queda para rutas, códigos de
disco y velocidades.

### Hierarchy
- **Display** (800, 44px, 1.15, -0.035em): sólo el nombre «Holocron» en Inicio.
- **Headline** (600, 30px, 1.15): el título de cada pantalla.
- **Numeral** (700, 24–30px, 1.2, -0.02em, cifras tabulares): el número de cada
  baldosa y de cada indicador. Es la voz principal del sistema.
- **Title** (600, 15–17px): títulos de panel y de sección.
- **Body** (400, 15px, 1.55): el texto corrido y las filas de datos.
- **Label** (600, 11–12px, 0.14–0.16em, mayúsculas): grupos del menú y
  encabezados de tabla. Los textos de apoyo bajo un número van en 12–13px sin
  mayúsculas.

### Named Rules
**The Tabular Numbers Rule.** Todo número que se actualiza o se compara lleva
cifras tabulares (`font-variant-numeric: tabular-nums`): los dígitos no bailan
cuando el valor cambia en vivo.

**The 11px Floor Rule.** Ningún texto baja de 11px: ni insignias, ni rótulos, ni
la etiqueta «GINEBRA». Lo que se lee (pies de póster, rutas, explicaciones) va en
12px como mínimo.

**The One Family Rule.** No se suma un webfont. La fuente del sistema es la de la
portada y no le agrega peso al binario.

## Layout

Una barra lateral fija de 236px (la marca y el menú en cuatro grupos: Ahora,
Biblioteca, Descargas, Equipo) y el contenido a la derecha, con un ancho máximo de
1240px y 32px de margen. Inicio cambia el modelo: el contenido se centra, como la
portada, con el nombre arriba y las baldosas en una grilla de hasta 980px.

En pantallas táctiles, todo lo que se toca mide al menos 44px de alto (links
del menú, botones, chips, campos); con mouse, los controles quedan más densos.

Las grillas se acomodan solas (`auto-fill`/`auto-fit` con mínimos de 140–340px
según el contenido) en vez de fijar columnas. La escala de espacios es de 4px (4,
8, 12, 16, 24, 32); los paneles respiran con 16–24px adentro y 16px entre sí.

Por debajo de 900px la barra lateral se vuelve una barra arriba que se desliza de
costado, y el contenido toma todo el ancho con 16px de margen. Los pósters van en
proporción 2:3: en filas que se deslizan de costado (138px cada uno) o en grillas
de al menos 140px.

## Elevation & Depth

Sin sombras. La profundidad sale de tres capas: el mural de pósters al fondo
(opacidad .34 en Inicio y .16 en el resto, bajo un velo que oscurece donde va el
contenido), los paneles de vidrio con desenfoque de 14px encima, y el borde fino
que recorta cada panel. Un panel se levanta sólo en el hover de una baldosa: sube
4px y su vidrio se aclara, sin sombra. Por debajo de 900px el vidrio se aplana a
la superficie sólida: ahí el velo deja casi nada del mural, y el desenfoque es
trabajo que el teléfono paga en cada cuadro del scroll.

### Named Rules
**The No-Shadow Rule.** Ningún `box-shadow` de profundidad. Si algo necesita
separarse, se resuelve con vidrio, con el borde fino o con el fondo; nunca con una
sombra. Las únicas excepciones son el resplandor violeta de la marca y la sombra
del póster en la cabecera de Actividad, que es parte de la imagen.

**The Glass-Over-Wall Rule.** El vidrio existe porque hay un mural detrás. Un
panel sin nada detrás se pinta con la superficie sólida, no con vidrio.

## Shapes

Esquinas suaves y constantes: 18px para los paneles y las baldosas, 12px para
botones, campos e íconos, 10px para los pósters y 6px para los detalles chicos.
Lo que es un estado (chips, insignias, barras) va en píldora. Los íconos son de
línea (1.75px, extremos redondeados) y van sobre cuadrados redondeados con un
degradado de dos facetas del G▶.

## Components

### Buttons
Precisos y livianos: contorno fino, sin relleno; la acción se nota sin gritar.
- **Shape:** esquinas suaves (12px).
- **Primary:** texto y borde violeta Ginebra sobre transparente, 14px, peso 500,
  padding de 8px por 14px.
- **Hover / Focus:** relleno violeta al 12 % (22 % al apretar), en .15s; foco con
  anillo violeta de 2px.
- **Secondary:** texto claro y borde fino; en hover, un velo claro al 7 %.
- **Danger:** texto rojo y borde rojo al 45 %; el hover lo lleva al 100 %. Es la
  única forma de las acciones irreversibles, y van con confirmación.
- **Icon:** 36px cuadrado, el mismo trato.

### Chips
- **Atención:** píldora de violeta sombra con texto lila claro y borde violeta al
  35 %. Linkea a donde se resuelve lo que avisa.
- **Lectura de estado:** píldora de vidrio con borde fino y cifras tabulares, con
  un punto verde cuando dice «en línea».

### Cards / Containers
- **Corner Style:** 18px.
- **Background:** vidrio (rgba(24, 29, 40, .72)) con desenfoque de 14px.
- **Shadow Strategy:** ninguna (ver Elevation & Depth).
- **Border:** 1px, blanco al 9 %.
- **Internal Padding:** 16px (24px en los bloques de Ajustes).

### Inputs / Fields
- **Style:** superficie sólida, borde fino, 12px, altura mínima de 36px, cursor
  violeta.
- **Focus:** el borde pasa a violeta Ginebra, sin anillo extra.
- **Error / Disabled:** el error va en un aviso rojo debajo, no en el campo; los
  deshabilitados bajan a opacidad .45.

### Navigation
- **Style:** barra lateral de vidrio oscuro con la marca arriba (el cubo con un
  resplandor violeta, «Holocron» y la etiqueta «GINEBRA» en lila faceta) y cuatro
  grupos con un rótulo en mayúsculas.
- **Links:** 14px con ícono de línea; en hover, un velo claro al 6 %; el activo
  lleva un fondo violeta al 14 % y el ícono en violeta Ginebra.
- **Mobile:** por debajo de 900px, una barra arriba que se desliza de costado,
  sin rótulos de grupo y sin el nombre al lado del cubo.

### Baldosa (signature component)
La unidad de Inicio: un área de Holocron que también es la puerta a su pantalla.
Ícono en degradado de dos facetas (rosa, violeta, verde, ámbar…), el título, un
número grande que dice si vale la pena entrar y una línea que lo explica. En hover
sube 4px y el borde se pone violeta. Si algo pide atención, el número pasa a lila
faceta y el borde también; si el área no está configurada, la baldosa baja a
opacidad .55.

### Movimiento
Poco y con sentido: el cambio de estado de botones y links (.15s), la baldosa que
sube al pasar el mouse (.18s), el spinner de un trabajo y el punto «en vivo»
que late. Con «reducir movimiento», lo que se mueve en el espacio se detiene y lo
que dice un estado lo sigue diciendo: el punto queda fijo, la baldosa cambia su
borde y su vidrio sin subir, y el spinner late en lugar de girar.

### Puertas a las apps
Jellyfin y Seerr, las dos que la casa usa todos los días, van como tarjetas de
vidrio grandes (logo de 48px, nombre, para qué sirve y un chevron); el resto del
stack, como chips en píldora con logo de 22px. Los dos suben 3px y prenden el
borde violeta al pasar el mouse. En la página Stack las dos principales se repiten
en grande, con su botón «Abrir», y el resto va en una grilla de tarjetas.

### Póster
Proporción 2:3, esquinas de 10px, carga diferida. Sin imagen, la inicial del
título sobre un degradado de superficie alta a violeta sombra; nunca un ícono de
imagen rota. En la cabecera de Actividad, el póster de lo que se reproduce aparece
dos veces: nítido, con su sombra, y desenfocado de fondo. El estado («reproduciendo»,
«en pausa») va como insignia junto a la posición, no como rótulo sobre el título.

## Do's and Don'ts

### Do:
- **Do** dar a cada panel un número que mande (24–30px, peso 700, cifras tabulares) y una línea que lo explique.
- **Do** usar el violeta Ginebra (#a78bfa) sólo en lo que se toca o está activo, y el lila faceta (#c084fc) para el texto en acento.
- **Do** separar con vidrio, borde fino (blanco al 9 %) y fondo; nunca con sombras.
- **Do** mostrar un póster vacío como la inicial sobre el degradado, nunca como imagen rota.
- **Do** resolver los anchos dinámicos con SVG o clases precomputadas: la CSP no deja `style=""`.

### Don't:
- **Don't** armar un panel de administración genérico: grillas de tarjetas grises iguales y tablas por todos lados.
- **Don't** rellenar los botones de color sólido: son de contorno fino.
- **Don't** agregar `box-shadow` para dar profundidad.
- **Don't** encender una luz de estado (verde, rojo, amarillo) cuando no dice nada.
- **Don't** sumar un webfont ni una segunda familia tipográfica.
- **Don't** poner un rótulo en mayúsculas encima de un título: el título habla solo, y un estado va como insignia al lado del dato que califica.
- **Don't** bajar un texto de 11px ni inventar otra opacidad para el texto secundario: hay dos, suave y tenue.
