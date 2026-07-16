export type AdminRequest = (
  path: string,
  options?: RequestInit,
) => Promise<any>;

export function adminClient(username: string, password: string): AdminRequest {
  const headers = () => ({
    Authorization: `Basic ${btoa(unescape(encodeURIComponent(`${username}:${password}`)))}`,
    "Content-Type": "application/json",
  });
  return async function request(path: string, options: RequestInit = {}) {
    const response = await fetch(`/api/admin/${path}`, {
      ...options,
      headers: { ...headers(), ...(options.headers || {}) },
    });
    if (response.status === 401) throw new Error("AUTH");
    if (!response.ok) {
      let message = await response.text();
      try {
        message = JSON.parse(message).error || message;
      } catch {}
      throw new Error(message);
    }
    return response.status === 204 ? null : response.json();
  };
}
