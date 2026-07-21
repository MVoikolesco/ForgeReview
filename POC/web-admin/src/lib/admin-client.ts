export type AdminRequest = <T>(
  path: string,
  options?: RequestInit,
) => Promise<T>;

function encodeCredentials(username: string, password: string) {
  const bytes = new TextEncoder().encode(`${username}:${password}`);
  let binary = "";
  bytes.forEach((byte) => (binary += String.fromCharCode(byte)));
  return btoa(binary);
}

export function createAdminClient(
  username: string,
  password: string,
): AdminRequest {
  return async function request<T>(path: string, options: RequestInit = {}) {
    const response = await fetch(`/api/admin/${path}`, {
      ...options,
      headers: {
        Authorization: `Basic ${encodeCredentials(username, password)}`,
        "Content-Type": "application/json",
        ...options.headers,
      },
    });

    if (response.status === 401) throw new Error("AUTH");
    if (!response.ok) {
      const text = await response.text();
      let message = text;
      try {
        const payload = JSON.parse(text) as {
          error?: string | { message?: string };
        };
        const error = payload.error;
        message = typeof error === "string" ? error : error?.message || text;
      } catch {
        // Keep the raw response when it is not JSON.
      }
      throw new Error(message || "Não foi possível concluir a operação.");
    }

    return (response.status === 204 ? null : await response.json()) as T;
  };
}
