import assert from "node:assert/strict";
import test from "node:test";
import {
  parseResponseSchemaText,
  responseSchemaError,
} from "./response-contracts";

test("accepts a supported Draft 2020-12 response schema", () => {
  assert.equal(
    responseSchemaError({
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "array",
      items: { type: "string" },
    }),
    undefined,
  );
});

test("rejects malformed schemas and external references", () => {
  const malformed = parseResponseSchemaText("{");
  assert.ok("error" in malformed);
  assert.match(malformed.error, /JSON válido/);
  assert.match(
    responseSchemaError({ $ref: "https://example.com/schema.json" }) ?? "",
    /externas/,
  );
  assert.match(responseSchemaError({ type: "unknown" }) ?? "", /inválido/);
});
