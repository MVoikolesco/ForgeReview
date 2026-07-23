export type Viewport = { width: number; height: number };
export type Rectangle = { left: number; top: number; right: number; bottom: number; width: number; height: number };
export type PopoverPlacement = { side: "right" | "left" | "bottom" | "top"; x: number; y: number };

const inset = 12;
const gap = 14;
const clamp = (value: number, minimum: number, maximum: number) =>
  Math.max(minimum, Math.min(value, Math.max(minimum, maximum)));

/** Places a card editor beside its anchor before falling back to the roomiest edge. */
export function placeCardEditor(
  anchor: Rectangle,
  viewport: Viewport,
  panel: { width: number; height: number },
): PopoverPlacement {
  const centeredY = anchor.top + anchor.height / 2 - panel.height / 2;
  const centeredX = anchor.left + anchor.width / 2 - panel.width / 2;
  const candidates: PopoverPlacement[] = [
    { side: "right", x: anchor.right + gap, y: centeredY },
    { side: "left", x: anchor.left - panel.width - gap, y: centeredY },
    { side: "bottom", x: centeredX, y: anchor.bottom + gap },
    { side: "top", x: centeredX, y: anchor.top - panel.height - gap },
  ];
  const fits = (candidate: PopoverPlacement) =>
    candidate.x >= inset && candidate.y >= inset &&
    candidate.x + panel.width <= viewport.width - inset &&
    candidate.y + panel.height <= viewport.height - inset;
  const preferred = candidates.find(fits) ?? candidates.reduce((best, candidate) => {
    const room = candidate.side === "right" ? viewport.width - anchor.right
      : candidate.side === "left" ? anchor.left
        : candidate.side === "bottom" ? viewport.height - anchor.bottom
          : anchor.top;
    const bestRoom = best.side === "right" ? viewport.width - anchor.right
      : best.side === "left" ? anchor.left
        : best.side === "bottom" ? viewport.height - anchor.bottom
          : anchor.top;
    return room > bestRoom ? candidate : best;
  });
  return {
    ...preferred,
    x: clamp(preferred.x, inset, viewport.width - panel.width - inset),
    y: clamp(preferred.y, inset, viewport.height - panel.height - inset),
  };
}
