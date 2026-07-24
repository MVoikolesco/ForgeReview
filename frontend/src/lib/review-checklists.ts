import type { ReviewChecklist, ReviewChecklistSnapshot } from "./types";

export const officialReviewChecklist: ReviewChecklist = {
  key: "official.pull-request.v1",
  name: "Review de pull request",
  description:
    "Checklist fechada para segurança, corretude, contratos, performance, arquitetura e observabilidade.",
  version: 1,
  editable: false,
  items: [
    { check_id: "security.authorization", description: "Verificar bypass de autenticação, autorização ou isolamento de dados introduzido pela alteração.", category: "security", minimum_context: "file" },
    { check_id: "correctness.behavior", description: "Verificar comportamento incorreto reproduzível no fluxo alterado, incluindo limites e tratamento de erro.", category: "correctness", minimum_context: "diff" },
    { check_id: "contracts.compatibility", description: "Verificar quebra concreta de contrato público, formato persistido, API ou integração.", category: "contracts", minimum_context: "contracts" },
    { check_id: "performance.hot_path", description: "Verificar regressão material em caminho executado com frequência ou crescimento não limitado.", category: "performance", minimum_context: "symbol" },
    { check_id: "architecture.boundaries", description: "Verificar violação funcional de fronteira que cria acoplamento ou efeito externo indevido.", category: "architecture", minimum_context: "dependencies" },
    { check_id: "observability.failures", description: "Verificar falha operacional nova que fica silenciosa ou perde sinal necessário para diagnóstico.", category: "observability", minimum_context: "file" },
  ],
};

export function checklistSnapshot(
  checklist: ReviewChecklist,
): ReviewChecklistSnapshot {
  return {
    key: checklist.key,
    name: checklist.name,
    version: checklist.version,
    items: checklist.items.map((item) => ({ ...item })),
  };
}

export function reviewChecklistError(value: unknown): string | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return "A checklist deve ser um objeto versionado.";
  const item = value as Partial<ReviewChecklistSnapshot>;
  if (
    typeof item.key !== "string" ||
    !item.key.trim() ||
    typeof item.name !== "string" ||
    !item.name.trim() ||
    !Number.isInteger(item.version) ||
    (item.version ?? 0) < 1 ||
    !Array.isArray(item.items) ||
    item.items.length < 1 ||
    item.items.length > 64
  )
    return "A checklist requer chave, nome, versão positiva e de 1 a 64 checks.";
  const ids = new Set<string>();
  const categories = new Set(["security", "correctness", "contracts", "performance", "architecture", "observability"]);
  const contexts = new Set(["diff", "file", "symbol", "dependencies", "tests", "contracts", "repository"]);
  for (const check of item.items) {
    if (
      !check ||
      typeof check !== "object" ||
      !/^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/.test(check.check_id) ||
      ids.has(check.check_id) ||
      !check.description?.trim() ||
      !categories.has(check.category) ||
      !contexts.has(check.minimum_context)
    )
      return "A checklist contém check_id duplicado ou item inválido.";
    ids.add(check.check_id);
  }
  return undefined;
}

