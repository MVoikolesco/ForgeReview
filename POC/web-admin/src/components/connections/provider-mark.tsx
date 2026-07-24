import { Box, Route } from "lucide-react";

export function ProviderMark({ provider, large = false }: { provider?: string; large?: boolean }) {
  return (
    <span className={`provider-mark ${provider || "generic"} ${large ? "large" : ""}`}>
      {provider === "ollama" ? <Box size={large ? 24 : 18} /> : <Route size={large ? 24 : 18} />}
    </span>
  );
}
