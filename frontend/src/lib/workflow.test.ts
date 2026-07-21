import assert from "node:assert/strict";
import test from "node:test";
import { normalizeCard } from "./api";
import {
  canConnect,
  canPublishVersion,
  workflowVersionStatusLabel,
} from "./workflow";

test("catalog cards without ports normalize to empty lists", () => {
  const card = normalizeCard({
    key: "trigger",
    name: "Trigger",
    category: "Entradas",
    description: "",
    inputs: null,
    outputs: null,
  } as never);
  assert.deepEqual(card.inputs, []);
  assert.deepEqual(card.outputs, []);
});

test("typed connection accepts matching and wildcard contracts", () => {
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "event", required: false },
      { key: "in", label: "In", contract: "event", required: true },
    ),
    true,
  );
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "any", required: false },
      { key: "in", label: "In", contract: "files", required: true },
    ),
    true,
  );
});

test("typed connection blocks missing and incompatible ports", () => {
  assert.equal(
    canConnect(undefined, {
      key: "in",
      label: "In",
      contract: "event",
      required: true,
    }),
    false,
  );
  assert.equal(
    canConnect(
      { key: "out", label: "Out", contract: "event", required: false },
      { key: "in", label: "In", contract: "files", required: true },
    ),
    false,
  );
});

test("workflow version lifecycle exposes publishable drafts only", () => {
  assert.equal(workflowVersionStatusLabel("draft"), "Rascunho");
  assert.equal(workflowVersionStatusLabel("published"), "Publicada");
  assert.equal(workflowVersionStatusLabel("archived"), "Arquivada");
  assert.equal(canPublishVersion("draft"), true);
  assert.equal(canPublishVersion("published"), false);
  assert.equal(canPublishVersion("archived"), false);
});
