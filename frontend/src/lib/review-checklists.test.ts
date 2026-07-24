import assert from "node:assert/strict";
import test from "node:test";
import {
  checklistSnapshot,
  officialReviewChecklist,
  reviewChecklistError,
} from "./review-checklists";

test("official checklist is valid and snapshots are detached", () => {
  assert.equal(reviewChecklistError(officialReviewChecklist), undefined);
  const snapshot = checklistSnapshot(officialReviewChecklist);
  assert.equal(snapshot.version, 1);
  assert.equal(snapshot.items.length, 6);
  assert.notEqual(snapshot.items, officialReviewChecklist.items);
});

test("checklist validation rejects duplicate check ids", () => {
  const snapshot = checklistSnapshot(officialReviewChecklist);
  snapshot.items.push({ ...snapshot.items[0] });
  assert.match(reviewChecklistError(snapshot) ?? "", /duplicado/);
});

