import assert from "node:assert/strict";
import test from "node:test";
import { pushHistory, redoHistory, searchCards, selectedCardSearchResult, studioLoadTarget, undoHistory } from "./studio";
import { placeCardEditor } from "./popover";
import { normalizeExecutionReport } from "./api";

const definition = (key: string) => ({ key, name: key, description: "", nodes: [], edges: [] });
test("serializable Studio history supports undo and redo without UI state", () => {
  const history = pushHistory({ past: [], future: [] }, definition("one"));
  const undo = undoHistory(history, definition("two"));
  assert.equal(undo?.definition.key, "one");
  const redo = redoHistory(undo!.history, undo!.definition);
  assert.equal(redo?.definition.key, "two");
});
test("card search matches name, key, and category", () => {
  assert.deepEqual(searchCards([{ key: "fetch", name: "Buscar PR", category: "Dados" }], "dados buscar").map(x => x.key), ["fetch"]);
});
test("card search activation uses the highlighted available result or first available match", () => {
  const cards = [{ key: "fetch" }, { key: "workflow", available: false }, { key: "model" }];
  assert.equal(selectedCardSearchResult(cards, "model")?.key, "model");
  assert.equal(selectedCardSearchResult(cards, "workflow")?.key, "fetch");
});
test("card editor placement stays in the viewport and flips away from a right-edge card", () => {
  const placement = placeCardEditor(
    { left: 820, top: 220, right: 940, bottom: 300, width: 120, height: 80 },
    { width: 1000, height: 700 },
    { width: 340, height: 420 },
  );
  assert.equal(placement.side, "left");
  assert.ok(placement.x >= 12 && placement.y >= 12);
  assert.ok(placement.x + 340 <= 988 && placement.y + 420 <= 688);
});
test("card editor placement clamps a constrained canvas without covering its anchor when space exists", () => {
  const placement = placeCardEditor(
    { left: 20, top: 40, right: 140, bottom: 120, width: 120, height: 80 },
    { width: 800, height: 600 },
    { width: 320, height: 300 },
  );
  assert.equal(placement.side, "right");
  assert.ok(placement.x >= 154);
});
test("Studio resolves explicit versions, named published workflows, and the official published default", () => {
  assert.deepEqual(studioLoadTarget("42", "other"), { kind: "version", versionID: 42 });
  assert.deepEqual(studioLoadTarget(null, "review"), { kind: "published", workflowKey: "review" });
  assert.deepEqual(studioLoadTarget(null, null), { kind: "published", workflowKey: "official-gitea-pr-review" });
});

test("incomplete queued and running reports retain safe status without iterable crashes", () => {
  for (const status of ["queued", "running"]) {
    const report = normalizeExecutionReport({ status, runs: null });
    assert.equal(report.status, status);
    assert.deepEqual(report.runs, []);
    assert.match(report.contractIssue || "", /progresso incompleto/);
  }
});
