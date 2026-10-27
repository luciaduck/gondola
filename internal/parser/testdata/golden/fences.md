---
title: Fenced blocks
---
# Fenced blocks

::: +note #welcome ?data-kind[intro]
Welcome to **Gondola**.

- a list
- inside a fence
:::

Text with an inline element:

:::span +highlight /
inline **content**
:::

:::: #wrapper
::: +inner
Nested *block*.
:::
::::

:::\note #tip
A note made from an alias.
:::

::::\gallery +wide
:::figure
![A gondola](gondola.jpg)
:::
::::

::: +warning /TODO: rewrite before launch
Comments after "/" are dropped.
:::

::://kept as an HTML comment

::: ?onclick["items[0].focus()"] ?title[two words]
Quoted values may contain "]".
:::
