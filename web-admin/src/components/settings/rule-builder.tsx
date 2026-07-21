"use client";

import {
  collectionItemFields,
  collectionItemSchema,
  defaultRule,
  schemaFields,
  type CollectionScope,
  type Rule,
  type RulePredicate,
  type RulePredicateOperator,
  type RuleValue,
  type SchemaField,
} from "./pipeline-types";

const predicateOperators: RulePredicateOperator[] = [
  "equals",
  "contains",
  "in",
  "gt",
  "gte",
  "lt",
  "lte",
  "matches",
  "exists",
];
const collectionScopes: CollectionScope[] = ["any", "all", "count", "filter"];

function operatorsFor(field?: SchemaField) {
  if (field?.type === "number" || field?.type === "integer")
    return ["equals", "in", "gt", "gte", "lt", "lte", "exists"] as const;
  if (field?.type === "boolean") return ["equals", "exists"] as const;
  return predicateOperators;
}

function parseValue(value: string, field?: SchemaField, operator?: string): RuleValue {
  const parseScalar = (item: string) => {
    const trimmed = item.trim();
    if (field?.type === "number" || field?.type === "integer") {
      const parsed = Number(trimmed);
      return Number.isFinite(parsed) ? parsed : 0;
    }
    if (field?.type === "boolean") return trimmed === "true";
    return trimmed;
  };
  return operator === "in" ? value.split(",").map(parseScalar) : parseScalar(value);
}

function valueText(value?: RuleValue) {
  return Array.isArray(value) ? value.join(", ") : String(value ?? "");
}

function RuleEditor({
  rule,
  fields,
  schema,
  disabled,
  depth,
  onChange,
  onRemove,
}: {
  rule: Rule;
  fields: SchemaField[];
  schema?: Record<string, unknown>;
  disabled: boolean;
  depth: number;
  onChange: (rule: Rule) => void;
  onRemove?: () => void;
}) {
  const changeKind = (operator: string) => {
    if (operator === "all" || operator === "any")
      onChange({ operator, rules: [defaultRule(fields)] });
    else if (operator === "not")
      onChange({ operator: "not", rules: [defaultRule(fields)] });
    else
      onChange({
        ...defaultRule(fields),
        operator: operator as RulePredicateOperator,
      });
  };

  if (rule.operator === "all" || rule.operator === "any")
    return (
      <div className="rule-node rule-group">
        <div className="rule-row">
          <select
            aria-label="Operador da regra"
            disabled={disabled}
            value={rule.operator}
            onChange={(event) => changeKind(event.target.value)}
          >
            <option value="all">all</option>
            <option value="any">any</option>
            <option value="not">not</option>
            {predicateOperators.map((operator) => (
              <option key={operator}>{operator}</option>
            ))}
          </select>
          {onRemove && (
            <button disabled={disabled} onClick={onRemove} type="button">
              Remover
            </button>
          )}
        </div>
        {rule.rules.map((child, index) => (
          <RuleEditor
            key={index}
            rule={child}
            fields={fields}
            schema={schema}
            disabled={disabled}
            depth={depth + 1}
            onChange={(next) =>
              onChange({
                ...rule,
                rules: rule.rules.map((item, itemIndex) =>
                  itemIndex === index ? next : item,
                ),
              })
            }
            onRemove={
              rule.rules.length > 1
                ? () =>
                    onChange({
                      ...rule,
                      rules: rule.rules.filter((_, itemIndex) => itemIndex !== index),
                    })
                : undefined
            }
          />
        ))}
        <button
          className="rule-add"
          disabled={disabled}
          onClick={() =>
            onChange({ ...rule, rules: [...rule.rules, defaultRule(fields)] })
          }
          type="button"
        >
          + condição
        </button>
      </div>
    );

  if (rule.operator === "not")
    return (
      <div className="rule-node rule-group">
        <div className="rule-row">
          <select
            aria-label="Operador da regra"
            disabled={disabled}
            value="not"
            onChange={(event) => changeKind(event.target.value)}
          >
            <option value="not">not</option>
            <option value="all">all</option>
            <option value="any">any</option>
            {predicateOperators.map((operator) => (
              <option key={operator}>{operator}</option>
            ))}
          </select>
          {onRemove && (
            <button disabled={disabled} onClick={onRemove} type="button">
              Remover
            </button>
          )}
        </div>
        <RuleEditor
          rule={rule.rules[0]}
          fields={fields}
          schema={schema}
          disabled={disabled}
          depth={depth + 1}
          onChange={(next) => onChange({ ...rule, rules: [next] })}
        />
      </div>
    );

  const selectedPath = rule.scope?.path ?? rule.path ?? "result";
  const field = fields.find((item) => item.path === selectedPath);
  const itemFields = rule.scope
    ? collectionItemFields(schema, rule.scope.path)
    : [];
  const itemSchema = rule.scope
    ? collectionItemSchema(schema, rule.scope.path)
    : undefined;
  const projectedField: SchemaField | undefined = rule.scope
    ? {
        path: rule.scope.path,
        type:
          rule.scope.kind === "count"
            ? "number"
            : rule.scope.kind === "filter"
              ? "array"
              : "boolean",
        collection: false,
      }
    : field;
  const operators = operatorsFor(projectedField);
  return (
    <div className="rule-node">
      <div className="rule-row">
        <select
          aria-label="Campo da regra"
          disabled={disabled}
          value={selectedPath}
          onChange={(event) => {
            const nextField = fields.find((item) => item.path === event.target.value);
            if (nextField?.type === "array") {
              const nested = collectionItemFields(schema, nextField.path);
              onChange(
                nested.length
                  ? {
                      operator: "equals",
                      value: true,
                      scope: {
                        kind: "any",
                        path: nextField.path,
                        rule: defaultRule(nested),
                      },
                    }
                  : { operator: "exists", path: nextField.path },
              );
            } else {
              onChange({ ...rule, path: event.target.value, scope: undefined });
            }
          }}
        >
          {!fields.length && <option value={selectedPath}>{selectedPath}</option>}
          {fields
            .filter((item) => !item.collection || item.type === "array")
            .map((item) => (
              <option key={item.path} value={item.path}>
                {item.path} · {item.type}
              </option>
            ))}
        </select>
        {onRemove && (
          <button disabled={disabled} onClick={onRemove} type="button">
            Remover
          </button>
        )}
      </div>
      <div className="rule-row">
        {rule.scope && (
          <select
            aria-label="Escopo da coleção"
            disabled={disabled}
            value={rule.scope.kind}
            onChange={(event) => {
              const kind = event.target.value as CollectionScope;
              const nestedRule =
                rule.scope!.rule ??
                (itemFields.length ? defaultRule(itemFields) : undefined);
              onChange({
                ...rule,
                operator:
                  kind === "count"
                    ? "gt"
                    : kind === "filter"
                      ? "exists"
                      : "equals",
                value: kind === "count" ? 0 : kind === "filter" ? undefined : true,
                scope: {
                  ...rule.scope!,
                  kind,
                  rule: kind === "count" ? rule.scope!.rule : nestedRule,
                },
              });
            }}
          >
            {collectionScopes.map((scope) => (
              <option
                disabled={scope !== "count" && !itemFields.length}
                key={scope}
              >
                {scope}
              </option>
            ))}
          </select>
        )}
        <select
          aria-label="Operador da regra"
          disabled={disabled}
          value={rule.operator}
          onChange={(event) => {
            const operator = event.target.value;
            if (operator === "all" || operator === "any" || operator === "not")
              changeKind(operator);
            else
              onChange({
                ...rule,
                operator: operator as RulePredicateOperator,
                ...(operator === "exists" ? { value: undefined } : {}),
              });
          }}
        >
          <option value="all">all</option>
          <option value="any">any</option>
          <option value="not">not</option>
          {operators.map((operator) => (
            <option key={operator}>{operator}</option>
          ))}
        </select>
        {rule.operator !== "exists" && (
          projectedField?.type === "boolean" ? (
            <select
              aria-label="Valor da regra"
              disabled={disabled}
              value={String(rule.value ?? false)}
              onChange={(event) =>
                onChange({ ...rule, value: event.target.value === "true" })
              }
            >
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          ) : (
            <input
              aria-label="Valor da regra"
              disabled={disabled}
              inputMode={
                projectedField?.type === "number" || projectedField?.type === "integer"
                  ? "decimal"
                  : undefined
              }
              placeholder={rule.operator === "in" ? "valor 1, valor 2" : "valor"}
              value={valueText(rule.value)}
              onChange={(event) =>
                onChange({
                  ...rule,
                  value: parseValue(event.target.value, projectedField, rule.operator),
                })
              }
            />
          )
        )}
      </div>
      {rule.scope && (
        <div className="rule-scope">
          <small>Filtro aplicado aos itens de {rule.scope.path}</small>
          {rule.scope.rule ? (
            <RuleEditor
              rule={rule.scope.rule}
              fields={itemFields}
              schema={itemSchema}
              disabled={disabled}
              depth={depth + 1}
              onChange={(next) =>
                onChange({ ...rule, scope: { ...rule.scope!, rule: next } })
              }
            />
          ) : (
            <button
              disabled={disabled}
              onClick={() =>
                onChange({
                  ...rule,
                  scope: { ...rule.scope!, rule: defaultRule(itemFields) },
                })
              }
              type="button"
            >
              + filtro
            </button>
          )}
        </div>
      )}
    </div>
  );
}

export function RuleBuilder({
  rule,
  schema,
  disabled = false,
  onChange,
}: {
  rule?: Rule;
  schema?: Record<string, unknown>;
  disabled?: boolean;
  onChange: (rule?: Rule) => void;
}) {
  const fields = schemaFields(schema);
  return (
    <div className="rule-builder">
      <div className="rule-builder-heading">
        <span>Regra dinâmica</span>
        <button
          disabled={disabled}
          onClick={() => onChange(rule ? undefined : defaultRule(fields))}
          type="button"
        >
          {rule ? "Usar always" : "Adicionar regra"}
        </button>
      </div>
      {rule ? (
        <RuleEditor
          rule={rule}
          fields={fields}
          schema={schema}
          disabled={disabled}
          depth={0}
          onChange={onChange}
        />
      ) : (
        <small>Sem AST: a condição compatível será usada.</small>
      )}
      {!fields.length && rule && (
        <small>Schema indisponível; mantendo o caminho atual com edição segura.</small>
      )}
    </div>
  );
}
