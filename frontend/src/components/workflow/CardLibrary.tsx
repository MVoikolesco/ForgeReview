import { Database, GitBranch, Layers3, LockKeyhole, Plus } from "lucide-react";
import type { CardType } from "../../lib/types";
import styles from "./CardLibrary.module.scss";

export function CardLibrary({
  cards,
  onAdd,
  readOnly = false,
}: {
  cards: CardType[];
  onAdd: (card: CardType) => void;
  readOnly?: boolean;
}) {
  const categories = [...new Set(cards.map((card) => card.category))];
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
      {categories.map((category) => (
        <section key={category}>
          <h2>
            {icon(category)} {category}
          </h2>
          {cards
            .filter((card) => card.category === category)
            .map((card) => (
              <button
                key={card.key}
                title={card.available === false ? card.unavailable_reason || "Card indisponível" : card.description}
                onClick={() => onAdd(card)} disabled={readOnly || card.available === false}
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
