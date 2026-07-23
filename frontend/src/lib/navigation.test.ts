import assert from "node:assert/strict";
import test from "node:test";
import { breadcrumbs, visibleNavigation } from "./navigation";

test("navigation only exposes privileged destinations to their roles", () => {
  assert.deepEqual(
    visibleNavigation("viewer").map((item) => item.href),
    ["/", "/pipelines"],
  );
  assert.deepEqual(
    visibleNavigation("editor").map((item) => item.href),
    ["/", "/pipelines", "/studio"],
  );
  assert.deepEqual(
    visibleNavigation("admin").map((item) => item.href),
    ["/", "/pipelines", "/studio", "/integrations", "/admin"],
  );
});

test("breadcrumbs retain a semantic overview return", () => {
  assert.deepEqual(breadcrumbs("/integrations", "Integrações"), [
    { label: "Visão geral", href: "/" },
    { label: "Integrações" },
  ]);
});
