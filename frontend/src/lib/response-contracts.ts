import Ajv2020 from "ajv/dist/2020";

const maxSchemaBytes = 64 << 10;
const maxSchemaDepth = 32;

export const candidateFindingsSchema: Record<string, unknown> = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  type: "array",
  items: {
    type: "object",
    additionalProperties: false,
    required: [
      "check_id",
      "claim",
      "scenario",
      "impact",
      "evidence",
      "confidence",
      "required_context",
      "symbol",
      "issue_type",
      "affected_entity",
      "path",
      "line",
      "comment",
      "severity",
    ],
    properties: {
      check_id: { type: "string", minLength: 1 },
      claim: { type: "string", minLength: 1 },
      scenario: { type: "string", minLength: 1 },
      impact: { type: "string", minLength: 1 },
      evidence: {
        type: "array",
        minItems: 1,
        items: { type: "string", minLength: 1 },
      },
      confidence: { type: "number", minimum: 0, maximum: 1 },
      required_context: {
        type: "array",
        items: { type: "string", minLength: 1 },
      },
      symbol: { type: "string", minLength: 1 },
      issue_type: { type: "string", minLength: 1 },
      affected_entity: { type: "string", minLength: 1 },
      path: { type: "string", minLength: 1 },
      line: { type: "integer", minimum: 1 },
      comment: { type: "string", minLength: 1 },
      severity: {
        type: "string",
        enum: ["low", "medium", "high", "critical"],
      },
    },
  },
};

export function responseSchemaError(value: unknown): string | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value))
    return "O contrato de resposta deve ser um objeto JSON Schema.";
  const serialized = JSON.stringify(value);
  if (new TextEncoder().encode(serialized).length > maxSchemaBytes)
    return `O contrato excede o limite de ${maxSchemaBytes} bytes.`;
  const structuralError = inspectSchema(value, 0);
  if (structuralError) return structuralError;
  try {
    const ajv = new Ajv2020({ allErrors: true, strict: false });
    ajv.compile(value);
    return undefined;
  } catch (error) {
    return `JSON Schema inválido: ${error instanceof Error ? error.message : "não foi possível compilar"}.`;
  }
}

function inspectSchema(value: unknown, depth: number): string | undefined {
  if (depth > maxSchemaDepth)
    return `O contrato excede a profundidade máxima de ${maxSchemaDepth}.`;
  if (Array.isArray(value)) {
    for (const nested of value) {
      const error = inspectSchema(nested, depth + 1);
      if (error) return error;
    }
    return undefined;
  }
  if (!value || typeof value !== "object") return undefined;
  for (const [key, nested] of Object.entries(value)) {
    if (
      key === "$ref" &&
      (typeof nested !== "string" || !nested.startsWith("#"))
    )
      return "Referências externas ($ref) não são permitidas.";
    const error = inspectSchema(nested, depth + 1);
    if (error) return error;
  }
  return undefined;
}

export function parseResponseSchemaText(
  text: string,
): { schema: Record<string, unknown> } | { error: string } {
  try {
    const schema = JSON.parse(text) as unknown;
    const error = responseSchemaError(schema);
    if (error) return { error };
    return { schema: schema as Record<string, unknown> };
  } catch {
    return { error: "O contrato não contém JSON válido." };
  }
}
