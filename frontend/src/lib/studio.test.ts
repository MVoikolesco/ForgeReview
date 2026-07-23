import assert from "node:assert/strict";
import test from "node:test";
import { pushHistory, redoHistory, searchCards, undoHistory } from "./studio";

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
