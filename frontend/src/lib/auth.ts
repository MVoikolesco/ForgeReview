import { APIError } from "./api";

export function loginErrorMessage(reason: unknown): string {
  if (reason instanceof APIError && reason.status === 401) return "O e-mail ou a senha não correspondem a uma conta local.";
  if (reason instanceof APIError && reason.status === 400) return "Confira o e-mail e a senha e tente novamente.";
  if (reason instanceof APIError && reason.status === undefined) return "O servidor ForgeReview não está disponível. Confirme que o backend está em execução.";
  return "Não foi possível autenticar neste momento. Tente novamente.";
}

export const sessionExpiredMessage = "Sua sessão expirou. Entre novamente para continuar.";
