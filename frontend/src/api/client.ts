// The single place that talks to the API. Everything else goes through the
// hooks in queries.ts, so a route change lands in one file.

import { getAccessToken } from "../auth/supabase";

/** ApiError carries the server's own message, which the handlers write to be
 *  read by a person ("type exactly \"DELETE ALL MY DATA\" to confirm"), so it
 *  is shown as-is rather than replaced with a generic failure notice. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<TResponse>(
  path: string,
  init?: RequestInit,
): Promise<TResponse> {
  // Read per request rather than once at module load: the token is refreshed
  // in the background, and a cached one goes stale mid-session.
  const accessToken = await getAccessToken();

  const headers: Record<string, string> = {};
  if (init?.body !== undefined) headers["Content-Type"] = "application/json";
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;

  const response = await fetch(`/api${path}`, {
    ...init,
    headers: { ...headers, ...(init?.headers as Record<string, string>) },
  });

  if (!response.ok) {
    let message = `request failed with status ${response.status}`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // A non-JSON error body leaves the status-code message in place.
    }
    throw new ApiError(response.status, message);
  }

  // 204 No Content is a success with nothing to parse.
  if (response.status === 204) return undefined as TResponse;
  return (await response.json()) as TResponse;
}

export const api = {
  get: <TResponse>(path: string) => request<TResponse>(path),

  post: <TResponse>(path: string, body?: unknown) =>
    request<TResponse>(path, {
      method: "POST",
      body: body === undefined ? undefined : JSON.stringify(body),
    }),

  put: <TResponse>(path: string, body?: unknown) =>
    request<TResponse>(path, {
      method: "PUT",
      body: body === undefined ? undefined : JSON.stringify(body),
    }),

  delete: <TResponse>(path: string) =>
    request<TResponse>(path, { method: "DELETE" }),
};

/** Builds a query string, omitting empty values so the API sees an absent
 *  parameter rather than an empty one. */
export function queryString(
  parameters: Record<string, string | number | boolean | undefined>,
): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(parameters)) {
    if (value === undefined || value === "") continue;
    search.set(key, String(value));
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : "";
}
