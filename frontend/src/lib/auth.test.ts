import assert from "node:assert/strict";
import test from "node:test";
import { APIError } from "./api";
import { loginErrorMessage } from "./auth";

test("login errors distinguish unavailable authentication from invalid credentials", () => {
  assert.equal(loginErrorMessage(new APIError("invalid credentials", 401)), "O e-mail ou a senha não correspondem a uma conta local.");
  assert.match(loginErrorMessage(new APIError("offline")), /backend está em execução/);
});
