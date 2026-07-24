import { Braces } from "lucide-react";

export function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className="brand">
      <span className="brand-mark"><Braces size={20} strokeWidth={2.4} /></span>
      {!compact && (
        <span className="brand-name">Forge<span>Review</span></span>
      )}
    </div>
  );
}
