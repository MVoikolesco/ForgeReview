import assert from "node:assert/strict";
import test from "node:test";
import { APIError } from "./api";
import { loginErrorMessage, sessionExpiredMessage } from "./auth";

test("login errors distinguish unavailable authentication from invalid credentials", () => {
  assert.equal(
    loginErrorMessage(new APIError("invalid credentials", 401)),
    "O e-mail ou a senha não correspondem a uma conta local.",
  );
  assert.match(
    loginErrorMessage(new APIError("offline")),
    /backend está em execução/,
  );
});

test("expired sessions have an explicit return-to-login message", () => {
  assert.match(sessionExpiredMessage, /sessão expirou/);
});
