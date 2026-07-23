"use client";

import { Database, GitBranch, Layers3, LockKeyhole, Plus, Search } from "lucide-react";
import { useState } from "react";
import type { CardType } from "../../lib/types";
import { searchCards } from "../../lib/studio";
import styles from "./CardLibrary.module.scss";

export function CardLibrary({
  cards,
  onAdd,
  onDragStart,
  readOnly = false,
}: {
  cards: CardType[];
  onAdd: (card: CardType) => void;
  onDragStart?: (card: CardType) => void;
  readOnly?: boolean;
}) {
  const [query, setQuery] = useState("");
  const filteredCards = searchCards(cards, query);
  const categories = [...new Set(filteredCards.map((card) => card.category))];
  const icon = (category: string) =>
    category === "Dados" ? (
      <Database size={14} />
    ) : category === "Controle" ? (
      <GitBranch size={14} />
    ) : (
      <Layers3 size={14} />
    );
  return (
    <aside className={styles.library} aria-label="Biblioteca de cards">
      <div className={styles.heading}>
        <b>Cards</b>
        <small>Adicione ao canvas</small>
      </div>
      <label className={styles.search}>
        <Search size={14} aria-hidden="true" />
        <span className="sr-only">Buscar cards</span>
        <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Buscar cards" />
      </label>
      {categories.map((category) => (
        <section key={category}>
          <h2>
            {icon(category)} {category}
          </h2>
          {filteredCards
            .filter((card) => card.category === category)
            .map((card) => (
              <button
                key={card.key}
                title={card.available === false ? card.unavailable_reason || "Card indisponível" : card.description}
                onClick={() => onAdd(card)} disabled={readOnly || card.available === false}
                draggable={!readOnly && card.available !== false}
                onDragStart={(event) => {
                  event.dataTransfer.setData("application/forgereview-card", card.key);
                  event.dataTransfer.effectAllowed = "move";
                  onDragStart?.(card);
                }}
                aria-describedby={card.available === false ? `card-${card.key}-availability` : undefined}
              >
                <i /> <span>{card.name}</span>
                {card.available === false ? <LockKeyhole size={13} /> : <Plus size={13} />}
                {card.available === false && <small id={`card-${card.key}-availability`}>Indisponível</small>}
              </button>
            ))}
        </section>
      ))}
    </aside>
  );
}
