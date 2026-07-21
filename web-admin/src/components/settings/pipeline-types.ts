import type { ReviewSettingsContract } from "@/lib/contracts";

export type RuleScalar = string | number | boolean | null;
export type RuleValue = RuleScalar | RuleScalar[];
export type RulePredicateOperator =
  | "equals"
  | "contains"
  | "in"
  | "gt"
  | "gte"
  | "lt"
  | "lte"
  | "matches"
  | "exists";
export type RuleLogicalOperator = "all" | "any" | "not";
export type RuleOperator = RulePredicateOperator | RuleLogicalOperator;
export type CollectionScope = "any" | "all" | "count" | "filter";

export type RuleCollectionScope = {
  kind: CollectionScope;
  path: string;
  rule?: Rule;
};

export type RulePredicate = {
  operator: RulePredicateOperator;
  path?: string;
  value?: RuleValue;
  scope?: RuleCollectionScope;
};

export type RuleGroup =
  | { operator: "all"; rules: Rule[] }
  | { operator: "any"; rules: Rule[] }
  | { operator: "not"; rules: [Rule] };

export type Rule = RulePredicate | RuleGroup;

export type RoutingMode = "all_matches" | "first_match";
export type JoinMode = "each_arrival" | "any" | "wait_all";

export type PipelineContract = Pick<ReviewSettingsContract, "key"> &
  Partial<Pick<ReviewSettingsContract, "schema">>;

export type PipelineStage = {
  id?: number;
  stage_type_key: string;
  executor_key?: string;
  processor_kind?: string;
  key: string;
  name: string;
  position?: number;
  prompt_template: string;
  model_id?: number;
  max_output_tokens: number;
  retry_limit: number;
  timeout_seconds: number;
  use_llm: boolean;
  required: boolean;
  route_mode?: RoutingMode;
  routing_mode?: RoutingMode;
  join_mode?: JoinMode;
  config: Record<string, unknown>;
  input_contract?: PipelineContract;
  output_contract?: PipelineContract;
};

export type PipelineTransition = {
  id?: number;
  from_stage_id?: number;
  to_stage_id?: number;
  from_stage_key?: string;
  to_stage_key?: string;
  type: string;
  condition_key: string;
  rule?: Rule;
  max_traversals?: number;
  priority: number;
};

export type SchemaField = {
  path: string;
  type: "string" | "number" | "integer" | "boolean" | "object" | "array";
  collection: boolean;
};

type JSONSchema = {
  type?: string | string[];
  properties?: Record<string, JSONSchema>;
  items?: JSONSchema;
};

function schemaType(schema: JSONSchema): SchemaField["type"] {
  const value = Array.isArray(schema.type)
    ? schema.type.find((item) => item !== "null")
    : schema.type;
  return value === "number" ||
    value === "integer" ||
    value === "boolean" ||
    value === "object" ||
    value === "array"
    ? value
    : "string";
}

export function schemaFields(schema?: Record<string, unknown>): SchemaField[] {
  const fields: SchemaField[] = [];
  const visit = (node: JSONSchema, prefix = "", collection = false) => {
    const type = schemaType(node);
    if (prefix && type !== "object")
      fields.push({ path: prefix, type, collection });

    if (type === "array" && node.items) {
      const itemType = schemaType(node.items);
      if (itemType !== "object" && prefix)
        fields.push({ path: prefix, type: itemType, collection: true });
      visit(node.items, prefix, true);
      return;
    }

    Object.entries(node.properties ?? {}).forEach(([key, child]) =>
      visit(child, prefix ? `${prefix}.${key}` : key, collection),
    );
  };

  if (schema) visit(schema as JSONSchema);
  return fields.filter(
    (field, index) =>
      fields.findIndex((candidate) => candidate.path === field.path) === index,
  );
}

export function defaultRule(fields: SchemaField[]): RulePredicate {
  const field = fields.find((item) => !item.collection || item.type === "array");
  return {
    operator: "exists",
    path: field?.path ?? "result",
  };
}

export function collectionItemFields(
  schema: Record<string, unknown> | undefined,
  path: string,
): SchemaField[] {
  let node = schema as JSONSchema | undefined;
  for (const part of path.split(".")) node = node?.properties?.[part];
  return node?.items ? schemaFields(node.items as Record<string, unknown>) : [];
}

export function ruleSummary(rule?: Rule): string {
  if (!rule) return "always";
  if (rule.operator === "all" || rule.operator === "any") {
    const separator = rule.operator === "all" ? " AND " : " OR ";
    const body = rule.rules.map(ruleSummary).join(separator) || "empty";
    return `(${body})`;
  }
  if (rule.operator === "not") return `NOT ${ruleSummary(rule.rules[0])}`;
  const scopedPath = rule.scope
    ? `${rule.scope.path}[${rule.scope.kind}]`
    : rule.path ?? "$";
  if (rule.operator === "exists") return `${scopedPath} exists`;
  const value = Array.isArray(rule.value)
    ? rule.value.join(", ")
    : String(rule.value ?? "");
  return `${scopedPath} ${rule.operator} ${value}`.trim();
}
