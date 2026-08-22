import { useState, type FormEvent } from "react";
import { isAuthConfigured, supabase } from "../auth/supabase";
import { Button, ErrorNotice, TextField } from "../components/ui";

/** Sign-in only, deliberately. Public signup is disabled on the Supabase
 *  project and there is no "create account" path here — an open registration
 *  form on a one-person app is a standing invitation. The single account is
 *  created by hand in the Supabase dashboard. */
export function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setErrorMessage(null);
    setIsSubmitting(true);

    const { error } = await supabase.auth.signInWithPassword({
      email: email.trim(),
      password,
    });

    if (error) {
      // Supabase deliberately does not say which of the two was wrong, and
      // neither does this: distinguishing them tells an attacker which email
      // addresses exist.
      setErrorMessage(error.message);
      setIsSubmitting(false);
      return;
    }
    // No navigation needed — the session listener swaps the whole app over.
  };

  return (
    <div className="grid min-h-dvh place-items-center bg-surface-sunk px-4">
      <div className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <span className="text-4xl" aria-hidden>
            🕯️
          </span>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight text-ink">
            Bat-ti
          </h1>
          <p className="mt-1 text-sm text-ink-soft">
            Raw material prices and spending
          </p>
        </div>

        <form
          onSubmit={submit}
          className="space-y-3 rounded-2xl border border-wick-100 bg-surface p-5 shadow-sm"
        >
          <TextField
            label="Email"
            type="email"
            value={email}
            onChange={setEmail}
            placeholder="you@example.com"
            required
          />
          <TextField
            label="Password"
            type="password"
            value={password}
            onChange={setPassword}
            required
          />

          {errorMessage && <ErrorNotice error={new Error(errorMessage)} />}

          {!isAuthConfigured && (
            <ErrorNotice
              error={
                new Error(
                  "Sign-in is not configured: set VITE_SUPABASE_URL and VITE_SUPABASE_PUBLISHABLE_KEY in frontend/.env.local, then restart the dev server.",
                )
              }
            />
          )}

          <Button
            type="submit"
            className="w-full"
            disabled={isSubmitting || !isAuthConfigured}
          >
            {isSubmitting ? "Signing in…" : "Sign in"}
          </Button>
        </form>
      </div>
    </div>
  );
}
