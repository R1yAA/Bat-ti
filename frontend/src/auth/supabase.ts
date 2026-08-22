import { createClient } from "@supabase/supabase-js";

// Both of these are meant to be public: the anon key is a client-side
// identifier that grants nothing on its own, and every private route is gated
// by the Go API verifying the signed token. The database password is a
// different thing entirely and must never appear in a VITE_ variable, because
// Vite inlines those into the bundle served to the browser.
const supabaseURL = import.meta.env.VITE_SUPABASE_URL as string | undefined;
const supabaseAnonKey = import.meta.env.VITE_SUPABASE_ANON_KEY as
  | string
  | undefined;

export const isAuthConfigured = Boolean(supabaseURL && supabaseAnonKey);

if (!isAuthConfigured) {
  console.warn(
    "VITE_SUPABASE_URL and VITE_SUPABASE_ANON_KEY are not set; sign-in is unavailable.",
  );
}

export const supabase = createClient(
  supabaseURL ?? "http://localhost",
  supabaseAnonKey ?? "anon",
  {
    auth: {
      persistSession: true,
      // The access token is short-lived; without this a session silently dies
      // mid-use and every request starts failing with a 401.
      autoRefreshToken: true,
      detectSessionInUrl: true,
    },
  },
);

/** The token to send with an API request, or null when signed out.
 *
 *  Read through getSession rather than cached in a module variable, so a
 *  token refreshed in the background is picked up by the very next request
 *  instead of the stale one being sent until a reload. */
export async function getAccessToken(): Promise<string | null> {
  if (!isAuthConfigured) return null;
  const { data } = await supabase.auth.getSession();
  return data.session?.access_token ?? null;
}
